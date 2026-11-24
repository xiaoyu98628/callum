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
	"sync"
	"unicode/utf8"

	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/tool"
)

const maxToolsPerServer = 128
const maxDiscoveryWorkers = 4

// toolCatalog 发布后只读，可以在其他调用占用连接时读取。
type toolCatalog struct {
	byName map[string]*remoteTool
	public []ToolInfo
}

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
	if ctx.Err() != nil {
		return nil
	}
	// 先识别可用目录，再用有界并发发现其余服务。等待某个服务不占用缓存读取。
	pending := make(chan *entry, len(entries))
	for _, e := range entries {
		if connect && cachedCatalog(e) == nil {
			pending <- e
		}
	}
	close(pending)
	var workers sync.WaitGroup
	for range min(maxDiscoveryWorkers, len(pending)) {
		workers.Go(func() {
			for e := range pending {
				if ctx.Err() != nil {
					return
				}
				s.ensureToolCatalog(ctx, e)
			}
		})
	}
	workers.Wait()

	s.mu.Lock()
	current := !s.closed && s.revision == revision
	s.mu.Unlock()
	if !current {
		return nil
	}
	var out []tool.Tool
	for _, e := range entries {
		if catalog := cachedCatalog(e); catalog != nil {
			for _, name := range e.cfg.AllowedTools {
				if t := catalog.byName[name]; t != nil {
					out = append(out, t)
				}
			}
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Definition().Name < out[b].Definition().Name })
	return out
}

func cachedCatalog(e *entry) *toolCatalog {
	if c := e.publicConnection.Load(); c != nil {
		if catalog := c.catalog.Load(); !c.dirty.Load() && e.ctx.Err() == nil && c.credentialVersion == e.credentialVersion.Load() {
			return catalog
		}
	}
	return nil
}

func (s *Service) ensureToolCatalog(ctx context.Context, e *entry) {
	if e.acquire(ctx) != nil {
		return
	}
	defer e.release()
	if ctx.Err() != nil || cachedCatalog(e) != nil {
		return
	}
	ctx, cancel := entryContext(ctx, e)
	defer cancel()
	c, err := s.connect(ctx, e)
	if err == nil && s.discover(ctx, e, c) != nil {
		s.closeConnection(e)
	}
}

func (s *Service) discover(ctx context.Context, e *entry, c *connection) error {
	c.catalog.Store(nil)
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
	tools := make([]ToolInfo, 0, len(next))
	for _, t := range next {
		tools = append(tools, t.info())
	}
	slices.SortFunc(tools, func(a, b ToolInfo) int { return strings.Compare(a.Name, b.Name) })
	c.catalog.Store(&toolCatalog{byName: next, public: tools})
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
		s.closeConnection(e)
		return notApplied, tool.ErrUnavailable
	}
	current := c.catalog.Load().byName[t.name]
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
