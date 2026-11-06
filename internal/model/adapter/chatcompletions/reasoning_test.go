package chatcompletions

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/model"
)

func TestReasoningResponseAndToolRoundTrip(t *testing.T) {
	for _, value := range []string{`"分析"`, `""`, `null`, `42`} {
		t.Run(value, func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(`{"choices":[{"index":0,"message":{"role":"assistant","content":"回答","reasoning_content":` + value + `},"finish_reason":"stop"}]}`)}
			client := newTestClient(t, func(req *http.Request) (*http.Response, error) {
				return testResponse(req, "application/json", body), nil
			})
			result, err := client.Generate(t.Context(), testRequest())
			if value == `42` {
				if !errors.Is(err, model.ErrInvalidResponse) {
					t.Fatalf("接受了无效推理字段: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			} else if value == `null` {
				if result.Reasoning != nil {
					t.Fatal("空字段被伪造为推理")
				}
			} else if result.Reasoning == nil {
				t.Fatal("显式推理字段丢失")
			}
			if body.closes != 1 {
				t.Fatal("响应未释放")
			}
		})
	}
	req := testRequest()
	req.Messages = append(req.Messages, model.Message{Role: model.RoleAssistant, Reasoning: new("分析"), ToolCalls: []model.ToolCall{{ID: "c", Name: "read_file", Arguments: json.RawMessage(`{}`)}}}, model.Message{Role: model.RoleTool, ToolCallID: "c", Content: "结果"})
	params, err := buildRequest(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"reasoning_content":"分析"`) {
		t.Fatalf("工具轮推理未回传: %s", raw)
	}
}

func TestReasoningStreamCallbackFailureClosesResponse(t *testing.T) {
	body := &trackedBody{Reader: strings.NewReader(sse(`{"choices":[{"index":0,"delta":{"reasoning_content":"分析"}}]}`, stopChunk, "[DONE]"))}
	client := newStreamClient(t, body)
	rejected := errors.New("controlled rejection")
	err := client.Stream(t.Context(), testRequest(), func(event model.Event) error {
		if event.Type == model.EventReasoningDelta {
			return rejected
		}
		if event.Type == model.EventCompleted {
			t.Error("回调失败被当作正常结束")
		}
		return nil
	})
	if !errors.Is(err, rejected) || body.closes != 1 {
		t.Fatalf("回调失败处理错误: %v closes=%d", err, body.closes)
	}
}
