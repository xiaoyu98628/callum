package chatcompletions

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/xiaoyu98628/callum/internal/model"
)

func TestToolDefinitionsAndMessages(t *testing.T) {
	req := model.Request{Model: "m", Tools: []model.ToolDefinition{{Name: "read_file", Description: "读取", Parameters: json.RawMessage(`{"type":"object"}`)}}, Messages: []model.Message{{Role: model.RoleUser, Content: "读取"}, {Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: "c", Name: "read_file", Arguments: json.RawMessage(`{"path":"a"}`)}}}, {Role: model.RoleTool, ToolCallID: "c", Content: `{"text":"ok"}`}}}
	params, err := buildRequest(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Tools    []json.RawMessage            `json:"tools"`
		Messages []map[string]json.RawMessage `json:"messages"`
		Choice   any                          `json:"tool_choice"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Tools) != 1 || len(data.Messages) != 3 || string(data.Messages[2]["tool_call_id"]) != `"c"` || data.Choice != nil {
		t.Fatalf("请求未保持自动选择和调用配对: %s", raw)
	}
}

func TestToolStreamAssemblyAndValidation(t *testing.T) {
	var capture streamCapture
	chunks := []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"c","type":"function","vendor":"call-meta","function":{"vendor":{"trace":"kept"},"name":"read_","arguments":"{\"path\":"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"file","arguments":"\"a\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`{"choices":[],"usage":{"total_tokens":9,"vendor":"kept"}}`,
	}
	for _, chunk := range chunks {
		if err := capture.add(chunk); err != nil {
			t.Fatal(err)
		}
	}
	result, err := capture.result("request")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(result.Output, []byte("call-meta")) || !bytes.Contains(result.Output, []byte("kept")) {
		t.Fatalf("工具扩展字段丢失: %s", result.Output)
	}
	calls, err := responseCalls(string(capture.messageJSON()), result.FinishReason, true)
	if err != nil || len(calls) != 1 || calls[0].Name != "read_file" || string(calls[0].Arguments) != `{"path":"a"}` || string(result.Usage) != `{"total_tokens":9,"vendor":"kept"}` {
		t.Fatalf("工具组装失败: %+v %s %v", calls, result.Usage, err)
	}
	for _, raw := range []string{`{"tool_calls":[{"id":"c","type":"function","function":{"name":"read_file","arguments":"{"}}]}`, `{"tool_calls":[{"id":"c","type":"function","function":{"name":"read_file","arguments":"{}"}},{"id":"c","type":"function","function":{"name":"read_file","arguments":"{}"}}]}`} {
		if _, err := responseCalls(raw, "tool_calls", true); !errors.Is(err, model.ErrInvalidResponse) {
			t.Fatalf("接受无效或重复调用: %v", err)
		}
	}
	if _, err := responseCalls(string(capture.messageJSON()), "tool_calls", false); !errors.Is(err, model.ErrUnsupportedOutput) {
		t.Fatal(err)
	}
}
