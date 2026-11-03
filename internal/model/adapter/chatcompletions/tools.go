package chatcompletions

import (
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"

	"github.com/xiaoyu98628/callum/internal/model"
)

const maxToolCalls = 32
const maxToolArguments = 16 << 10

var toolNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func addTools(params *openai.ChatCompletionNewParams, defs []model.ToolDefinition) error {
	if len(defs) > maxToolCalls {
		return model.ErrInputLimit
	}
	seen := map[string]bool{}
	for _, d := range defs {
		var schema map[string]any
		if !toolNamePattern.MatchString(d.Name) || seen[d.Name] || json.Unmarshal(d.Parameters, &schema) != nil || schema == nil {
			return model.ErrInvalidResponse
		}
		seen[d.Name] = true
		params.Tools = append(params.Tools, openai.ChatCompletionToolUnionParam{OfFunction: &openai.ChatCompletionFunctionToolParam{Function: shared.FunctionDefinitionParam{Name: d.Name, Description: openai.String(d.Description), Parameters: shared.FunctionParameters(schema)}}})
	}
	// 不指定必选工具，使用服务端默认的自动工具选择。
	return nil
}

func checkToolFinishReason(reason string, enabled bool) error {
	if reason == "tool_calls" && enabled {
		return nil
	}
	return checkFinishReason(reason)
}

func responseCalls(raw, reason string, enabled bool) ([]model.ToolCall, error) {
	var message struct {
		Calls []struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	}
	if json.Unmarshal([]byte(raw), &message) != nil {
		return nil, model.ErrInvalidResponse
	}
	if reason == "tool_calls" && !enabled {
		return nil, model.ErrUnsupportedOutput
	}
	if len(message.Calls) == 0 {
		if reason == "tool_calls" {
			return nil, model.ErrInvalidResponse
		}
		return nil, nil
	}
	if !enabled {
		return nil, model.ErrUnsupportedOutput
	}
	if reason != "tool_calls" || len(message.Calls) > maxToolCalls {
		return nil, model.ErrInvalidResponse
	}
	out := make([]model.ToolCall, 0, len(message.Calls))
	seen := map[string]bool{}
	for _, c := range message.Calls {
		var object map[string]json.RawMessage
		if c.ID == "" || len(c.ID) > 256 || seen[c.ID] || c.Type != "function" || !toolNamePattern.MatchString(c.Function.Name) || len(c.Function.Arguments) > maxToolArguments || json.Unmarshal([]byte(c.Function.Arguments), &object) != nil || object == nil {
			return nil, model.ErrInvalidResponse
		}
		seen[c.ID] = true
		out = append(out, model.ToolCall{ID: c.ID, Name: c.Function.Name, Arguments: json.RawMessage(c.Function.Arguments)})
	}
	return out, nil
}

type callFragment struct {
	ID, Name, Arguments strings.Builder
	Kind                string
}
type toolCallCapture struct {
	calls   map[int]*callFragment
	size    int
	unknown map[string][]json.RawMessage
}

func (c *toolCallCapture) add(raw json.RawMessage) error {
	var list []struct {
		Index    *int    `json:"index"`
		ID       *string `json:"id"`
		Type     *string `json:"type"`
		Function *struct {
			Name      *string `json:"name"`
			Arguments *string `json:"arguments"`
		} `json:"function"`
	}
	if json.Unmarshal(raw, &list) != nil {
		return model.ErrInvalidResponse
	}
	if c.calls == nil {
		c.calls = map[int]*callFragment{}
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return err
	}
	for position, d := range list {
		if d.Index == nil || *d.Index < 0 || *d.Index >= maxToolCalls {
			return model.ErrInvalidResponse
		}
		if err := c.captureUnknown(entries[position], *d.Index); err != nil {
			return err
		}
		f := c.calls[*d.Index]
		if f == nil {
			f = &callFragment{}
			c.calls[*d.Index] = f
		}
		if d.Type != nil {
			if *d.Type != "function" || (f.Kind != "" && f.Kind != *d.Type) {
				return model.ErrUnsupportedOutput
			}
			f.Kind = *d.Type
		}
		if d.ID != nil {
			f.ID.WriteString(*d.ID)
			c.size += len(*d.ID)
		}
		if d.Function != nil {
			if d.Function.Name != nil {
				f.Name.WriteString(*d.Function.Name)
				c.size += len(*d.Function.Name)
			}
			if d.Function.Arguments != nil {
				f.Arguments.WriteString(*d.Function.Arguments)
				c.size += len(*d.Function.Arguments)
			}
		}
		if f.ID.Len() > 256 || f.Name.Len() > 64 || f.Arguments.Len() > maxToolArguments || c.size > maxToolCalls*(maxToolArguments+320) {
			return model.ErrInputLimit
		}
	}
	return nil
}

func (c *toolCallCapture) result() (json.RawMessage, error) {
	if len(c.calls) == 0 {
		return nil, nil
	}
	keys := make([]int, 0, len(c.calls))
	for k := range c.calls {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	out := make([]map[string]any, 0, len(keys))
	for i, k := range keys {
		if i != k {
			return nil, model.ErrInvalidResponse
		}
		f := c.calls[k]
		out = append(out, map[string]any{"id": f.ID.String(), "type": f.Kind, "function": map[string]string{"name": f.Name.String(), "arguments": f.Arguments.String()}})
	}
	return json.Marshal(out)
}

// validateToolHistory 保证调用与结果完整配对，防止后续请求携带悬空或重复工具结果。
func validateToolHistory(messages []model.Message) error {
	pending := map[string]bool{}
	for _, m := range messages {
		if m.Role != model.RoleTool && len(pending) > 0 {
			return model.ErrInvalidResponse
		}
		if len(m.ToolCalls) > 0 {
			if m.Role != model.RoleAssistant {
				return model.ErrInvalidResponse
			}
			for _, c := range m.ToolCalls {
				var args map[string]json.RawMessage
				if c.ID == "" || len(c.ID) > 256 || pending[c.ID] || !toolNamePattern.MatchString(c.Name) || len(c.Arguments) > maxToolArguments || json.Unmarshal(c.Arguments, &args) != nil || args == nil {
					return model.ErrInvalidResponse
				}
				pending[c.ID] = true
			}
		}
		if m.Role == model.RoleTool {
			if !pending[m.ToolCallID] {
				return model.ErrInvalidResponse
			}
			delete(pending, m.ToolCallID)
		} else if m.ToolCallID != "" {
			return model.ErrInvalidResponse
		}
	}
	if len(pending) > 0 {
		return model.ErrInvalidResponse
	}
	return nil
}

// 未知字段按属性保留片段，不猜测供应商增量合并规则，也不保存完整 SSE 网络包。
func (c *toolCallCapture) captureUnknown(entry map[string]json.RawMessage, index int) error {
	if c.unknown == nil {
		c.unknown = map[string][]json.RawMessage{}
	}
	prefix := "tool_calls." + strconv.Itoa(index) + "."
	for key, value := range entry {
		switch key {
		case "index", "id", "type":
			continue
		case "function":
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(value, &fields); err != nil {
				return err
			}
			for name, part := range fields {
				if name != "name" && name != "arguments" {
					path := prefix + "function." + name
					c.unknown[path] = append(c.unknown[path], part)
				}
			}
		default:
			path := prefix + key
			c.unknown[path] = append(c.unknown[path], value)
		}
	}
	return nil
}
