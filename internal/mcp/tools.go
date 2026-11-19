package mcp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/tool"
)

const maxToolsPerServer = 128

type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type remoteTool struct {
	credentialVersion uint64
	service           *Service
	entry             *entry
	name              string
	version           string
	def               tool.Definition
	validator         *jsonschema.Resolved
}

func (t *remoteTool) info() ToolInfo {
	description := t.def.Description
	// 列表一次返回所有服务，缩短展示文案以保留 RPC 帧余量。
	if len(description) > 256 {
		end := 256
		for !utf8.RuneStart(description[end]) {
			end--
		}
		description = description[:end] + "…"
	}
	return ToolInfo{Name: t.name, Description: description}
}
func (t *remoteTool) Definition() tool.Definition { return t.def }

func (t *remoteTool) Validate(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) > tool.MaxArgumentBytes {
		return nil, tool.ErrLimit
	}
	if !json.Valid(raw) {
		return nil, tool.ErrInvalid
	}
	var obj map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&obj) != nil || obj == nil || t.validator.Validate(obj) != nil {
		return nil, tool.ErrInvalid
	}
	return json.Marshal(obj)
}

func (t *remoteTool) Prepare(ctx context.Context, _ tool.Scope, args json.RawMessage) (tool.Invocation, error) {
	if t.entry.ctx.Err() != nil || t.credentialVersion != t.entry.credentialVersion.Load() {
		return nil, tool.ErrUnavailable
	}
	s := t.entry.cfg
	target := s.Command
	if s.Transport == settings.MCPHTTP {
		u, _ := url.Parse(s.URL)
		u.RawQuery = ""
		target = u.String()
	}
	details, _ := json.Marshal(map[string]any{
		"kind": "mcp", "server_id": s.ID, "server_name": s.Name, "tool_name": t.name,
		"transport": s.Transport, "target": target, "definition_version": t.version,
	})
	return &invocation{tool: t, args: args, details: details}, nil
}

type invocation struct {
	tool    *remoteTool
	args    json.RawMessage
	details json.RawMessage
}

func (*invocation) Target() *tool.Target        { return nil }
func (*invocation) Close() error                { return nil }
func (i *invocation) Snapshot() json.RawMessage { return i.details }
func (i *invocation) Execute(ctx context.Context) (tool.Result, error) {
	return i.tool.service.call(ctx, i.tool, i.args)
}

// ToolsSnapshot 冻结供本轮模型选择的工具。预览只读取已发现的定义。
func (s *Service) ToolsSnapshot(ctx context.Context, connect bool, revision int64) []tool.Tool {
	s.mu.Lock()
	if s.revision != revision || s.closed {
		s.mu.Unlock()
		return nil
	}
	var entries []*entry
	for _, cfg := range s.cfg.Servers {
		if cfg.Enabled && len(cfg.AllowedTools) > 0 {
			entries = append(entries, s.entries[cfg.ID])
		}
	}
	s.mu.Unlock()
	var out []tool.Tool
	for _, e := range entries {
		if ctx.Err() != nil {
			break
		}
		if connect {
			if e.acquire(ctx) != nil {
				continue
			}
		} else {
			select {
			case e.lock <- struct{}{}:
			default:
				continue
			}
		}
		out = append(out, s.toolsForEntry(ctx, e, connect)...)
		e.release()
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Definition().Name < out[b].Definition().Name })
	return out
}

func (s *Service) toolsForEntry(ctx context.Context, e *entry, connect bool) []tool.Tool {
	if e.ctx.Err() != nil {
		return nil
	}
	c := e.connection
	if connect {
		ctx, cancel := entryContext(ctx, e)
		defer cancel()
		var err error
		if c, err = s.connect(ctx, e); err != nil {
			return nil
		}
		if c.tools == nil || c.dirty.Load() {
			if s.discover(ctx, e, c) != nil {
				closeConnection(e)
				return nil
			}
		}
	}
	if c == nil || c.dirty.Load() {
		return nil
	}
	var out []tool.Tool
	for _, name := range e.cfg.AllowedTools {
		if t := c.tools[name]; t != nil {
			out = append(out, t)
		}
	}
	return out
}

func (s *Service) discover(ctx context.Context, e *entry, c *connection) error {
	c.dirty.Store(false)
	next := map[string]*remoteTool{}
	if cap := c.session.InitializeResult().Capabilities; cap == nil || cap.Tools == nil {
		publishTools(c, next)
		return nil
	}
	seen := map[string]bool{}
	cursor := ""
	for range 32 {
		result, err := c.session.ListTools(ctx, &sdk.ListToolsParams{Cursor: cursor})
		if err != nil {
			return ErrConnection
		}
		for _, r := range result.Tools {
			if r == nil || seen[r.Name] || len(seen) >= maxToolsPerServer {
				return ErrConnection
			}
			seen[r.Name] = true
			if t := s.adapt(e, r); t != nil {
				next[r.Name] = t
			}
		}
		if cursor = result.NextCursor; cursor == "" {
			publishTools(c, next)
			return nil
		}
	}
	return ErrConnection
}

func publishTools(c *connection, next map[string]*remoteTool) {
	c.tools = next
	tools := make([]ToolInfo, 0, len(next))
	for _, t := range next {
		tools = append(tools, t.info())
	}
	slices.SortFunc(tools, func(a, b ToolInfo) int { return strings.Compare(a.Name, b.Name) })
	if !c.dirty.Load() {
		c.publicTools.Store(&tools)
	}
}

func (s *Service) adapt(e *entry, r *sdk.Tool) *remoteTool {
	if r.Name == "" || len(r.Name) > 256 || strings.ContainsAny(r.Name, "\x00\r\n") || !utf8.ValidString(r.Name) {
		return nil
	}
	input, err := json.Marshal(r.InputSchema)
	if err != nil || len(input) > tool.MaxArgumentBytes {
		return nil
	}
	var schema jsonschema.Schema
	if json.Unmarshal(input, &schema) != nil || schema.Type != "object" {
		return nil
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return nil
	}
	description := strings.TrimSpace(r.Description)
	if description == "" {
		description = "调用 " + r.Name
	}
	if len(description) > 4096 {
		return nil
	}
	fingerprint := sha256.Sum256(slices.Concat([]byte(description), []byte{0}, input))
	nameSum := sha256.Sum256([]byte(e.cfg.ID + "\x00" + r.Name))
	version := hex.EncodeToString(fingerprint[:])
	return &remoteTool{credentialVersion: e.credentialVersion.Load(), service: s, entry: e, name: r.Name, version: version, validator: resolved, def: tool.Definition{
		ToolDefinition: model.ToolDefinition{Name: "mcp_" + hex.EncodeToString(nameSum[:14]), Description: e.cfg.Name + " · " + description, Parameters: input},
		Source:         "mcp", Version: version, Effects: []tool.Effect{tool.EffectUnknown},
	}}
}

func (s *Service) call(ctx context.Context, t *remoteTool, args json.RawMessage) (tool.Result, error) {
	e := t.entry
	notApplied := tool.Result{Outcome: tool.OutcomeNotApplied}
	if !e.cfg.Enabled || !slices.Contains(e.cfg.AllowedTools, t.name) || e.credentialVersion.Load() != t.credentialVersion {
		return notApplied, tool.ErrDenied
	}
	if err := e.acquire(ctx); err != nil {
		return notApplied, errors.Join(tool.ErrUnavailable, err)
	}
	defer e.release()
	ctx, cancel := entryContext(ctx, e)
	defer cancel()
	c, err := s.connect(ctx, e)
	if err != nil {
		return notApplied, tool.ErrUnavailable
	}
	// 获批后重新获取定义，避免静默更新导致旧参数发往不同工具。
	if err := s.discover(ctx, e, c); err != nil {
		closeConnection(e)
		return notApplied, tool.ErrUnavailable
	}
	current := c.tools[t.name]
	if current == nil || current.version != t.version || t.credentialVersion != c.credentialVersion || c.credentialVersion != e.credentialVersion.Load() || c.dirty.Load() || ctx.Err() != nil {
		return notApplied, tool.ErrDenied
	}
	result, err := c.session.CallTool(ctx, &sdk.CallToolParams{Name: t.name, Arguments: args})
	if err != nil {
		return tool.Result{Outcome: tool.OutcomeUnknown}, tool.ErrUnknownOutcome
	}
	output, err := json.Marshal(result)
	if err != nil {
		return tool.Result{Outcome: tool.OutcomeUnknown}, tool.ErrUnknownOutcome
	}
	if result.IsError {
		// 工具错误可能发生在副作用之后，保留远端结果并明确待核实。
		return tool.Result{Output: output, Outcome: tool.OutcomeUnknown}, tool.ErrUnknownOutcome
	}
	return tool.Result{Output: output, Outcome: tool.OutcomeApplied}, nil
}
