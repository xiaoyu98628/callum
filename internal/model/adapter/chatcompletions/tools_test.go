package chatcompletions

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/model"
)

func TestToolDefinitionsAndMessages(t *testing.T) {
	req := model.Request{Model: "m", Tools: []model.ToolDefinition{{Name: "read_file", Description: "读取", Parameters: json.RawMessage(`{"type":"object"}`)}}, Messages: []model.Message{{Role: model.RoleUser, Content: "读取"}, {Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: "c", Name: "read_file", Arguments: json.RawMessage(`{"path":"a"}`)}}}, {Role: model.RoleTool, ToolCallID: "c", Content: `{"text":"ok"}`}}}
	params, err := (&Client{}).buildRequest(t.Context(), req)
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

func TestToolDefinitionsHaveNoLocalCountLimit(t *testing.T) {
	req := testRequest()
	for i := range 64 {
		req.Tools = append(req.Tools, model.ToolDefinition{Name: fmt.Sprintf("tool_%d", i), Parameters: json.RawMessage(`{"type":"object"}`)})
	}
	params, err := (&Client{}).buildRequest(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	var sent struct {
		Tools []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(raw, &sent); err != nil {
		t.Fatal(err)
	}
	if len(sent.Tools) != 64 {
		t.Fatalf("工具定义被拒绝或截断: %d", len(sent.Tools))
	}
	// 去掉数量阈值仍须拒绝重复定义及非法名称。
	req.Tools = append(req.Tools, req.Tools[0])
	if _, err := (&Client{}).buildRequest(t.Context(), req); !errors.Is(err, model.ErrInvalidResponse) {
		t.Fatalf("接受重复工具: %v", err)
	}
}

func TestLargeToolCallBatchForGenerateAndStream(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			calls := make([]map[string]any, 40)
			for i := range calls {
				calls[i] = map[string]any{"id": fmt.Sprintf("call-%d", i), "type": "function", "function": map[string]string{"name": "read_file", "arguments": `{"path":"a.txt"}`}}
				if stream {
					calls[i]["index"] = i
				}
			}
			choice := map[string]any{"index": 0, "finish_reason": "tool_calls"}
			if stream {
				choice["delta"] = map[string]any{"tool_calls": calls}
			} else {
				choice["message"] = map[string]any{"role": "assistant", "tool_calls": calls}
			}
			payload, err := json.Marshal(map[string]any{"choices": []any{choice}})
			if err != nil {
				t.Fatal(err)
			}
			wire := string(payload)
			if stream {
				wire = sse(wire, "[DONE]")
			}
			body := &trackedBody{Reader: strings.NewReader(wire)}
			client := newTestClient(t, func(req *http.Request) (*http.Response, error) {
				contentType := "application/json"
				if stream {
					contentType = "text/event-stream"
				}
				return testResponse(req, contentType, body), nil
			})
			req := testRequest()
			req.Tools = []model.ToolDefinition{{Name: "read_file", Parameters: []byte(`{"type":"object"}`)}}
			var response model.Response
			if stream {
				err = client.Stream(t.Context(), req, func(event model.Event) error {
					if event.Result != nil {
						response = *event.Result
					}
					return nil
				})
			} else {
				response, err = client.Generate(t.Context(), req)
			}
			if err != nil || len(response.ToolCalls) != 40 || body.closes != 1 {
				t.Fatalf("大批调用被截断或资源未释放: calls=%d closes=%d err=%v", len(response.ToolCalls), body.closes, err)
			}
			for i, call := range response.ToolCalls {
				if call.ID != fmt.Sprintf("call-%d", i) || string(call.Arguments) != `{"path":"a.txt"}` {
					t.Fatal("调用顺序或参数丢失")
				}
			}
		})
	}
}

func TestToolCallByteAndStructureBoundariesRemain(t *testing.T) {
	var capture toolCallCapture
	// 单个 index 不决定调用配额，但缺口、负值、重复 ID 和过大的参数仍应拒绝。
	if err := capture.add([]byte(`[{"index":40,"id":"sparse","type":"function","function":{"name":"read_file","arguments":"{}"}}]`)); err != nil {
		t.Fatal(err)
	}
	if _, err := capture.result(); !errors.Is(err, model.ErrInvalidResponse) {
		t.Fatal("接受缺口索引")
	}
	if err := capture.add([]byte(`[{"index":-1}]`)); !errors.Is(err, model.ErrInvalidResponse) {
		t.Fatal("接受负索引")
	}
	args := `{"content":"` + strings.Repeat("x", maxToolArguments) + `"}`
	call := map[string]any{"id": "large", "type": "function", "function": map[string]string{"name": "read_file", "arguments": args}}
	raw, err := json.Marshal(map[string]any{"tool_calls": []any{call}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := responseCalls(string(raw), "tool_calls", true); err == nil {
		t.Fatal("取消数量限制时放开了参数字节边界")
	}
	// 累计字段字节边界对完整响应与流式分片一致，单项参数均在合法范围内。
	call["function"] = map[string]string{"name": "read_file", "arguments": `{"content":"` + strings.Repeat("x", 16000) + `"}`}
	calls := make([]any, 525)
	for i := range calls {
		calls[i] = map[string]any{"id": fmt.Sprintf("c-%d", i), "type": call["type"], "function": call["function"]}
	}
	raw, err = json.Marshal(map[string]any{"tool_calls": calls})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := responseCalls(string(raw), "tool_calls", true); !errors.Is(err, model.ErrInputLimit) {
		t.Fatalf("完整调用总字节边界失效: %v", err)
	}
	var fragments toolCallCapture
	for i := range calls {
		calls[i].(map[string]any)["index"] = i
		raw, err := json.Marshal([]any{calls[i]})
		if err != nil {
			t.Fatal(err)
		}
		err = fragments.add(raw)
		if errors.Is(err, model.ErrInputLimit) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Fatal("流式调用总字节边界失效")
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

func TestInvalidToolJSONPreservesWholeResponseWithoutCalls(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			bad := `{"["string""]</｜｜DSML｜｜: `
			calls := []map[string]any{
				{"id": "valid", "type": "function", "function": map[string]string{"name": "read_file", "arguments": `{"path":"a.txt"}`}},
				{"id": "invalid", "type": "function", "function": map[string]string{"name": "read_file", "arguments": bad}},
			}
			message := map[string]any{"role": "assistant", "content": "准备读取", "tool_calls": calls}
			choice := map[string]any{"index": 0, "finish_reason": "tool_calls"}
			if stream {
				for i := range calls {
					calls[i]["index"] = i
				}
				choice["delta"] = message
			} else {
				choice["message"] = message
			}
			encoded, _ := json.Marshal(map[string]any{"choices": []any{choice}, "usage": map[string]int{"prompt_tokens": 13, "total_tokens": 17}})
			wire := string(encoded)
			if stream {
				wire = sse(wire, "[DONE]")
			}
			body := &trackedBody{Reader: strings.NewReader(wire)}
			client := newTestClient(t, func(req *http.Request) (*http.Response, error) {
				kind := "application/json"
				if stream {
					kind = "text/event-stream"
				}
				return testResponse(req, kind, body), nil
			})
			req := testRequest()
			req.Tools = []model.ToolDefinition{{Name: "read_file", Parameters: []byte(`{"type":"object"}`)}}
			var response model.Response
			var err error
			completed := false
			if stream {
				err = client.Stream(t.Context(), req, func(event model.Event) error {
					if event.Result != nil {
						response = *event.Result
					}
					if event.Type == model.EventCompleted {
						completed = true
					}
					return nil
				})
			} else {
				response, err = client.Generate(t.Context(), req)
			}
			detail, ok := errors.AsType[*model.ToolArgumentsError](err)
			if !ok || detail.Index != 1 || detail.Fingerprint == "" || len(response.ToolCalls) != 0 || response.FinishReason != "tool_calls" || !bytes.Contains(response.Output, []byte("DSML")) || response.InputTokens == nil || *response.InputTokens != 13 || body.closes != 1 || completed {
				t.Fatalf("坏参数未完整保留或被当作成功: response=%+v close=%d err=%v", response, body.closes, err)
			}
		})
	}
	// 身份错误与资源越界优先，不能借坏参数把损坏协议转换成可重试错误。
	for _, raw := range []string{
		`{"tool_calls":[{"id":"same","type":"function","function":{"name":"read_file","arguments":"{"}},{"id":"same","type":"function","function":{"name":"read_file","arguments":"{}"}}]}`,
		`{"tool_calls":[{"id":"bad","type":"function","function":{"name":"invalid name","arguments":"{"}}]}`,
	} {
		_, err := responseCalls(raw, "tool_calls", true)
		if _, ok := errors.AsType[*model.ToolArgumentsError](err); ok || !errors.Is(err, model.ErrInvalidResponse) {
			t.Fatalf("错误的工具身份被当作可纠正参数: %v", err)
		}
	}
}
