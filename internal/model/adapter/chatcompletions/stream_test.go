package chatcompletions

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/model"
)

const textChunk = `{"choices":[{"index":0,"delta":{"content":"你好"},"finish_reason":null}]}`
const stopChunk = `{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`
const trailingUsageChunk = `{"choices":[{"index":0,"delta":{}}],"usage":{"completion_tokens":115,"prompt_tokens":290,"total_tokens":405}}`

func sse(events ...string) string {
	return "data: " + strings.Join(events, "\n\ndata: ") + "\n\n"
}

func newStreamClient(t *testing.T, body *trackedBody) *Client {
	t.Helper()
	return newTestClient(t, func(req *http.Request) (*http.Response, error) {
		var wire struct {
			Stream bool `json:"stream"`
		}
		if err := json.NewDecoder(req.Body).Decode(&wire); err != nil {
			return nil, err
		}
		if !wire.Stream {
			t.Error("流式请求未设置 stream=true")
		}
		return testResponse(req, "text/event-stream", body), nil
	})
}

type fragmentReader struct {
	io.Reader
	size int
}

func (r fragmentReader) Read(p []byte) (int, error) {
	return r.Reader.Read(p[:min(len(p), r.size)])
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestStreamHandlesFragmentedUTF8AndClosesBeforeCompletion(t *testing.T) {
	body := &trackedBody{Reader: fragmentReader{
		Reader: strings.NewReader(sse(textChunk, stopChunk, `{"choices":[],"usage":{"total_tokens":2}}`, "[DONE]")),
		size:   1,
	}}
	client := newStreamClient(t, body)
	var events []model.Event
	err := client.Stream(t.Context(), testRequest(), func(event model.Event) error {
		if event.Type == model.EventCompleted && body.closes != 1 {
			t.Error("完成事件发送时响应尚未关闭")
		}
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []model.Event{
		{Type: model.EventTextDelta, Text: "你好", RequestID: testRequestID},
		{Type: model.EventCompleted, RequestID: testRequestID},
	}
	if !slices.Equal(events, want) {
		t.Errorf("事件为 %+v，期望 %+v", events, want)
	}
	if body.closes != 1 {
		t.Errorf("响应关闭次数为 %d，期望 1", body.closes)
	}
}

func TestStreamHandlesTrailingUsageChoice(t *testing.T) {
	for _, usageChunk := range []string{
		trailingUsageChunk,
		`{"choices":[{"index":0,"delta":{},"finish_reason":null}],"usage":{"total_tokens":405}}`,
	} {
		t.Run(usageChunk, func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(sse(textChunk, stopChunk, usageChunk, "[DONE]"))}
			client := newStreamClient(t, body)
			var events []model.Event
			err := client.Stream(t.Context(), testRequest(), func(event model.Event) error {
				if event.Type == model.EventCompleted && body.closes != 1 {
					t.Error("完成事件发送时响应尚未关闭")
				}
				events = append(events, event)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			want := []model.Event{
				{Type: model.EventTextDelta, Text: "你好", RequestID: testRequestID},
				{Type: model.EventCompleted, RequestID: testRequestID},
			}
			if !slices.Equal(events, want) {
				t.Errorf("事件为 %+v，期望 %+v", events, want)
			}
			if body.closes != 1 {
				t.Errorf("响应关闭次数为 %d，期望 1", body.closes)
			}
		})
	}
}

func TestStreamCallbackErrorsKeepRequestMetadata(t *testing.T) {
	for _, eventType := range []model.EventType{model.EventTextDelta, model.EventCompleted} {
		t.Run(string(eventType), func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(sse(textChunk, stopChunk, "[DONE]"))}
			client := newStreamClient(t, body)
			callbackErr := errors.New("测试回调失败")
			completed := false
			err := client.Stream(t.Context(), testRequest(), func(event model.Event) error {
				completed = completed || event.Type == model.EventCompleted
				if event.Type == eventType {
					return callbackErr
				}
				return nil
			})
			if !errors.Is(err, callbackErr) {
				t.Fatalf("回调错误链丢失：%v", err)
			}
			assertCallError(t, err, http.StatusOK, testRequestID)
			if eventType == model.EventTextDelta && completed {
				t.Error("内容回调失败后仍发送完成事件")
			}
			if body.closes != 1 {
				t.Errorf("响应关闭次数为 %d，期望 1", body.closes)
			}
		})
	}
}

func TestStreamRejectsInterruptedOrInvalidResponses(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		wantErr error
	}{
		{name: "未收到结束原因", payload: sse(textChunk, "[DONE]"), wantErr: io.ErrUnexpectedEOF},
		{name: "直接断流", payload: sse(textChunk), wantErr: io.ErrUnexpectedEOF},
		{name: "长度截断", payload: sse(`{"choices":[{"index":0,"delta":{},"finish_reason":"length"}]}`, "[DONE]"), wantErr: model.ErrIncomplete},
		{name: "结束后继续输出", payload: sse(stopChunk, textChunk, "[DONE]"), wantErr: model.ErrInvalidResponse},
		{name: "暂不支持工具输出", payload: sse(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call-test","type":"function","function":{"name":"lookup","arguments":"{"}}]},"finish_reason":null}]}`, "[DONE]"), wantErr: model.ErrUnsupportedOutput},
		{name: "无效JSON", payload: sse(textChunk, "{"), wantErr: nil},
		{name: "缺失delta", payload: sse(`{"choices":[{"index":0,"finish_reason":"stop"}]}`, "[DONE]"), wantErr: model.ErrInvalidResponse},
		{name: "空delta", payload: sse(`{"choices":[{"index":0,"delta":null,"finish_reason":"stop"}]}`, "[DONE]"), wantErr: model.ErrInvalidResponse},
		{name: "文本类型错误", payload: sse(`{"choices":[{"index":0,"delta":{"content":123},"finish_reason":null}]}`, stopChunk, "[DONE]"), wantErr: model.ErrInvalidResponse},
		{name: "拒绝内容类型错误", payload: sse(`{"choices":[{"index":0,"delta":{"refusal":123},"finish_reason":null}]}`, stopChunk, "[DONE]"), wantErr: model.ErrInvalidResponse},
		{name: "缺失索引", payload: sse(`{"choices":[{"delta":{},"finish_reason":"stop"}]}`, "[DONE]"), wantErr: model.ErrInvalidResponse},
		{name: "索引类型错误", payload: sse(`{"choices":[{"index":"0","delta":{},"finish_reason":"stop"}]}`, "[DONE]"), wantErr: model.ErrInvalidResponse},
		{name: "结束标识类型错误", payload: sse(`{"choices":[{"index":0,"delta":{},"finish_reason":123}]}`, "[DONE]"), wantErr: model.ErrInvalidResponse},
		{name: "缺失choices", payload: sse("{}", stopChunk, "[DONE]"), wantErr: model.ErrInvalidResponse},
		{name: "choices类型错误", payload: sse(`{"choices":{}}`, stopChunk, "[DONE]"), wantErr: model.ErrInvalidResponse},
		{name: "拒绝输出", payload: sse(`{"choices":[{"index":0,"delta":{"content":null,"refusal":"无法回答"},"finish_reason":null}]}`, "[DONE]"), wantErr: model.ErrRefused},
		{name: "用量不能替代结束原因", payload: sse(textChunk, trailingUsageChunk, "[DONE]"), wantErr: io.ErrUnexpectedEOF},
		{name: "长度截断后附加用量", payload: sse(`{"choices":[{"index":0,"delta":{},"finish_reason":"length"}]}`, trailingUsageChunk, "[DONE]"), wantErr: model.ErrIncomplete},
		{name: "结束后无用量的空choice", payload: sse(stopChunk, `{"choices":[{"index":0,"delta":{}}]}`, "[DONE]"), wantErr: model.ErrInvalidResponse},
		{name: "结束后用量为null", payload: sse(stopChunk, `{"choices":[{"index":0,"delta":{}}],"usage":null}`, "[DONE]"), wantErr: model.ErrInvalidResponse},
		{name: "结束后用量类型错误", payload: sse(stopChunk, `{"choices":[{"index":0,"delta":{}}],"usage":[]}`, "[DONE]"), wantErr: model.ErrInvalidResponse},
		{name: "结束后用量携带文本", payload: sse(stopChunk, `{"choices":[{"index":0,"delta":{"content":"继续生成"}}],"usage":{"total_tokens":405}}`, "[DONE]"), wantErr: model.ErrInvalidResponse},
		{name: "结束后用量携带拒绝内容", payload: sse(stopChunk, `{"choices":[{"index":0,"delta":{"refusal":"拒绝"}}],"usage":{"total_tokens":405}}`, "[DONE]"), wantErr: model.ErrInvalidResponse},
		{name: "结束后用量携带工具调用", payload: sse(stopChunk, `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call-test","type":"function","function":{"name":"lookup","arguments":"{}"}}]}}],"usage":{"total_tokens":405}}`, "[DONE]"), wantErr: model.ErrInvalidResponse},
		{name: "结束后用量携带未知增量字段", payload: sse(stopChunk, `{"choices":[{"index":0,"delta":{"audio":{}}}],"usage":{"total_tokens":405}}`, "[DONE]"), wantErr: model.ErrInvalidResponse},
		{name: "结束后用量修改结束原因", payload: sse(stopChunk, `{"choices":[{"index":0,"delta":{},"finish_reason":"length"}],"usage":{"total_tokens":405}}`, "[DONE]"), wantErr: model.ErrInvalidResponse},
		{name: "结束后用量重复结束原因", payload: sse(stopChunk, `{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"total_tokens":405}}`, "[DONE]"), wantErr: model.ErrInvalidResponse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(tt.payload)}
			client := newStreamClient(t, body)
			completed := false
			err := client.Stream(t.Context(), testRequest(), func(event model.Event) error {
				completed = completed || event.Type == model.EventCompleted
				return nil
			})
			if err == nil || (tt.wantErr != nil && !errors.Is(err, tt.wantErr)) {
				t.Fatalf("错误为 %v，期望 %v", err, tt.wantErr)
			}
			assertCallError(t, err, http.StatusOK, testRequestID)
			if completed {
				t.Error("异常流仍发送完成事件")
			}
			if body.closes != 1 {
				t.Errorf("响应关闭次数为 %d，期望 1", body.closes)
			}
		})
	}
}

func TestStreamAllowsEmptyAndNullableTextFields(t *testing.T) {
	body := &trackedBody{Reader: strings.NewReader(sse(
		`{"choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		`{"choices":[{"index":0,"delta":{"content":null,"refusal":null,"tool_calls":null},"finish_reason":null}]}`,
		`{"choices":[{"index":0,"delta":{"content":""},"finish_reason":null}]}`,
		textChunk, stopChunk, "[DONE]",
	))}
	client := newStreamClient(t, body)
	var events []model.Event
	err := client.Stream(t.Context(), testRequest(), func(event model.Event) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []model.Event{
		{Type: model.EventTextDelta, Text: "你好", RequestID: testRequestID},
		{Type: model.EventCompleted, RequestID: testRequestID},
	}
	if !slices.Equal(events, want) {
		t.Errorf("事件为 %+v，期望 %+v", events, want)
	}
	if body.closes != 1 {
		t.Errorf("响应关闭次数为 %d，期望 1", body.closes)
	}
}

func TestStreamCancellationReleasesResponse(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	body := &trackedBody{Reader: strings.NewReader(sse(textChunk, stopChunk, "[DONE]"))}
	client := newStreamClient(t, body)
	completed := false
	err := client.Stream(ctx, testRequest(), func(event model.Event) error {
		if event.Type == model.EventTextDelta {
			cancel()
		}
		completed = completed || event.Type == model.EventCompleted
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("取消错误链丢失：%v", err)
	}
	assertCallError(t, err, http.StatusOK, testRequestID)
	if completed || body.closes != 1 {
		t.Errorf("取消后的状态为 completed=%t, closes=%d", completed, body.closes)
	}
}

func TestStreamPreservesReadAndCloseErrors(t *testing.T) {
	readErr := errors.New("测试读取失败")
	closeErr := errors.New("测试关闭失败")
	for _, payload := range []string{sse(textChunk), sse(textChunk, stopChunk, trailingUsageChunk)} {
		t.Run(payload, func(t *testing.T) {
			body := &trackedBody{
				Reader:   io.MultiReader(strings.NewReader(payload), errorReader{err: readErr}),
				closeErr: closeErr,
			}
			client := newStreamClient(t, body)
			err := client.Stream(t.Context(), testRequest(), func(event model.Event) error {
				if event.Type == model.EventCompleted {
					t.Error("读取失败后仍发送完成事件")
				}
				return nil
			})
			if !errors.Is(err, readErr) || !errors.Is(err, closeErr) {
				t.Fatalf("读取或清理错误链丢失：%v", err)
			}
			if body.closes != 1 {
				t.Errorf("响应关闭次数为 %d，期望 1", body.closes)
			}
		})
	}
}

func TestStreamCloseFailurePreventsCompletion(t *testing.T) {
	closeErr := errors.New("测试关闭失败")
	body := &trackedBody{
		Reader:   strings.NewReader(sse(textChunk, stopChunk, "[DONE]")),
		closeErr: closeErr,
	}
	client := newStreamClient(t, body)
	err := client.Stream(t.Context(), testRequest(), func(event model.Event) error {
		if event.Type == model.EventCompleted {
			t.Error("关闭失败后仍发送完成事件")
		}
		return nil
	})
	if !errors.Is(err, closeErr) {
		t.Fatalf("关闭错误链丢失：%v", err)
	}
	assertCallError(t, err, http.StatusOK, testRequestID)
	if body.closes != 1 {
		t.Errorf("响应关闭次数为 %d，期望 1", body.closes)
	}
}
