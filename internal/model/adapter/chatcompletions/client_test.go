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
	"time"

	"github.com/openai/openai-go/v3"

	"github.com/xiaoyu98628/callum/internal/model"
)

const testRequestID = "req-test"

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (response *http.Response, err error) {
	defer func() {
		if req.Body != nil {
			err = errors.Join(err, req.Body.Close())
		}
	}()
	return f(req)
}

type trackedBody struct {
	io.Reader
	closes   int
	closeErr error
}

func (b *trackedBody) Close() error {
	b.closes++
	return b.closeErr
}

func newTestClient(t *testing.T, transport roundTripFunc) *Client {
	t.Helper()
	client, err := New(Config{
		BaseURL: "https://model.invalid/v1",
		APIKey:  "test-api-key",
	}, &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func testResponse(req *http.Request, contentType string, body io.ReadCloser) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": {contentType},
			"X-Request-Id": {testRequestID},
		},
		Body:    body,
		Request: req,
	}
}

func testRequest() model.Request {
	return model.Request{
		Model: "remote-model",
		Messages: []model.Message{
			{Role: model.RoleUser, Content: "你好"},
		},
	}
}

func assertCallError(t *testing.T, err error, statusCode int, requestID string) *model.CallError {
	t.Helper()
	callErr, ok := errors.AsType[*model.CallError](err)
	if !ok {
		t.Fatalf("错误类型为 %T，期望 *model.CallError", err)
	}
	if callErr.StatusCode != statusCode || callErr.RequestID != requestID {
		t.Errorf("错误元信息为 status=%d, request_id=%q，期望 status=%d, request_id=%q",
			callErr.StatusCode, callErr.RequestID, statusCode, requestID)
	}
	return callErr
}

func TestGenerateConvertsMessagesAndReleasesResponse(t *testing.T) {
	body := &trackedBody{Reader: strings.NewReader(`{"choices":[{"index":0,"message":{"role":"assistant","content":"你好"},"finish_reason":"stop"}]}`)}
	input := model.Request{
		Model: "remote-model",
		Messages: []model.Message{
			{Role: model.RoleSystem, Content: "用中文回答"},
			{Role: model.RoleUser, Content: "你好"},
			{Role: model.RoleAssistant, Content: "你好"},
		},
	}
	client := newTestClient(t, func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.Path != "/v1/chat/completions" {
			t.Errorf("请求为 %s %s", req.Method, req.URL.Path)
		}
		if req.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Error("认证头未使用传入的测试凭据")
		}
		var wire struct {
			Model    string          `json:"model"`
			Messages []model.Message `json:"messages"`
			Stream   bool            `json:"stream"`
		}
		if err := json.NewDecoder(req.Body).Decode(&wire); err != nil {
			return nil, err
		}
		if wire.Model != input.Model || !slices.Equal(wire.Messages, input.Messages) || wire.Stream {
			t.Errorf("请求转换错误：%+v", wire)
		}
		return testResponse(req, "application/json", body), nil
	})

	result, err := client.Generate(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "你好" || result.RequestID != testRequestID {
		t.Errorf("响应为 %+v", result)
	}
	if body.closes != 1 {
		t.Errorf("响应关闭次数为 %d，期望 1", body.closes)
	}
}

func TestGenerateRejectsInvalidResponses(t *testing.T) {
	for _, payload := range []string{
		"null",
		`{"choices":[]}`,
		`{"choices":[{"index":1}]}`,
		`{"choices":[{"index":0,"finish_reason":"stop"}]}`,
		`{"choices":[{"index":0,"message":null,"finish_reason":"stop"}]}`,
		`{"choices":[{"index":0,"message":[],"finish_reason":"stop"}]}`,
		`{"choices":[{"index":0,"message":{},"finish_reason":"stop"}]}`,
		`{"choices":[{"index":0,"message":{"content":null},"finish_reason":"stop"}]}`,
		`{"choices":[{"index":0,"message":{"content":123},"finish_reason":"stop"}]}`,
		`{"choices":[{"index":0,"message":{"content":[]},"finish_reason":"stop"}]}`,
		`{"choices":[{"index":0,"message":{"content":"ok","refusal":123},"finish_reason":"stop"}]}`,
		`{"choices":[{"index":0,"message":{"content":"ok","tool_calls":{}},"finish_reason":"stop"}]}`,
		`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`,
		`{"choices":[{"index":"0","message":{"content":"ok"},"finish_reason":"stop"}]}`,
		`{"choices":[{"index":0,"message":{"content":"ok"},"finish_reason":123}]}`,
	} {
		t.Run(payload, func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(payload)}
			client := newTestClient(t, func(req *http.Request) (*http.Response, error) {
				return testResponse(req, "application/json", body), nil
			})
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Errorf("异常响应触发 panic：%v", recovered)
				}
				if body.closes != 1 {
					t.Errorf("响应关闭次数为 %d，期望 1", body.closes)
				}
			}()

			_, err := client.Generate(t.Context(), testRequest())
			if !errors.Is(err, model.ErrInvalidResponse) {
				t.Fatalf("错误为 %v，期望 ErrInvalidResponse", err)
			}
			assertCallError(t, err, http.StatusOK, testRequestID)
		})
	}
}

func TestGenerateRetainsPartialText(t *testing.T) {
	body := &trackedBody{Reader: strings.NewReader(`{"choices":[{"index":0,"message":{"content":"尚未完成"},"finish_reason":"length"}]}`)}
	client := newTestClient(t, func(req *http.Request) (*http.Response, error) {
		return testResponse(req, "application/json", body), nil
	})
	result, err := client.Generate(t.Context(), testRequest())
	if !errors.Is(err, model.ErrIncomplete) {
		t.Fatalf("错误为 %v，期望 ErrIncomplete", err)
	}
	if result.Text != "尚未完成" || result.RequestID != testRequestID {
		t.Errorf("部分响应丢失：%+v", result)
	}
	if body.closes != 1 {
		t.Errorf("响应关闭次数为 %d，期望 1", body.closes)
	}
}

func TestGenerateRejectsToolCalls(t *testing.T) {
	body := &trackedBody{Reader: strings.NewReader(`{"choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"call-test","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"stop"}]}`)}
	client := newTestClient(t, func(req *http.Request) (*http.Response, error) {
		return testResponse(req, "application/json", body), nil
	})
	_, err := client.Generate(t.Context(), testRequest())
	if !errors.Is(err, model.ErrUnsupportedOutput) {
		t.Fatalf("工具输出被当作文本响应：%v", err)
	}
	assertCallError(t, err, http.StatusOK, testRequestID)
	if body.closes != 1 {
		t.Errorf("响应关闭次数为 %d，期望 1", body.closes)
	}
}

func TestGeneratePropagatesCallerContext(t *testing.T) {
	type contextKey struct{}
	ctx, cancel := context.WithCancel(context.WithValue(t.Context(), contextKey{}, "caller"))
	defer cancel()
	client := newTestClient(t, func(req *http.Request) (*http.Response, error) {
		if req.Context().Value(contextKey{}) != "caller" {
			t.Error("调用方上下文丢失")
		}
		cancel()
		return nil, req.Context().Err()
	})
	_, err := client.Generate(ctx, testRequest())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("取消错误链丢失：%v", err)
	}
}

func TestGenerateHonorsExpiredDeadline(t *testing.T) {
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	client := newTestClient(t, func(req *http.Request) (*http.Response, error) {
		return nil, req.Context().Err()
	})
	_, err := client.Generate(ctx, testRequest())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("超时错误链丢失：%v", err)
	}
}

func TestNewIgnoresEnvironmentOptions(t *testing.T) {
	t.Setenv("OPENAI_BASE_URL", "https://environment.invalid/v1")
	t.Setenv("OPENAI_API_KEY", "environment-api-key")
	t.Setenv("OPENAI_ADMIN_KEY", "environment-admin-key")
	t.Setenv("OPENAI_ORG_ID", "environment-org")
	t.Setenv("OPENAI_PROJECT_ID", "environment-project")
	t.Setenv("OPENAI_CUSTOM_HEADERS", "X-Environment: environment-value\nAuthorization: Bearer environment-key")

	for _, apiKey := range []string{"test-api-key", ""} {
		t.Run(apiKey, func(t *testing.T) {
			client, err := New(Config{BaseURL: "https://model.invalid/v1", APIKey: apiKey}, &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					if req.URL.Host != "model.invalid" || req.URL.Path != "/v1/chat/completions" {
						t.Error("请求地址未使用显式配置")
					}
					wantAuth := ""
					if apiKey != "" {
						wantAuth = "Bearer " + apiKey
					}
					if req.Header.Get("Authorization") != wantAuth {
						t.Error("认证头未使用显式配置")
					}
					for _, header := range []string{"OpenAI-Organization", "OpenAI-Project", "X-Environment"} {
						if req.Header.Get(header) != "" {
							t.Errorf("请求继承了环境请求头 %s", header)
						}
					}
					var wire struct {
						Stream bool `json:"stream"`
					}
					if err := json.NewDecoder(req.Body).Decode(&wire); err != nil {
						return nil, err
					}
					if wire.Stream {
						return testResponse(req, "text/event-stream", &trackedBody{
							Reader: strings.NewReader(sse(textChunk, stopChunk, "[DONE]")),
						}), nil
					}
					return testResponse(req, "application/json", &trackedBody{
						Reader: strings.NewReader(`{"choices":[{"index":0,"message":{"content":"ok"},"finish_reason":"stop"}]}`),
					}), nil
				}),
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.Generate(t.Context(), testRequest()); err != nil {
				t.Fatal(err)
			}
			if err := client.Stream(t.Context(), testRequest(), func(model.Event) error { return nil }); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGeneratePreservesOutputClassification(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		wantErr error
	}{
		{name: "空文本", payload: `{"choices":[{"index":0,"message":{"content":""},"finish_reason":"stop"}]}`},
		{name: "拒绝输出", payload: `{"choices":[{"index":0,"message":{"content":null,"refusal":"无法回答"},"finish_reason":"stop"}]}`, wantErr: model.ErrRefused},
		{name: "工具结束标识", payload: `{"choices":[{"index":0,"message":{"content":null},"finish_reason":"tool_calls"}]}`, wantErr: model.ErrUnsupportedOutput},
		{name: "长度截断", payload: `{"choices":[{"index":0,"message":{"content":null},"finish_reason":"length"}]}`, wantErr: model.ErrIncomplete},
		{name: "缺失结束标识", payload: `{"choices":[{"index":0,"message":{"content":"ok"}}]}`, wantErr: io.ErrUnexpectedEOF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(tt.payload)}
			client := newTestClient(t, func(req *http.Request) (*http.Response, error) {
				return testResponse(req, "application/json", body), nil
			})
			_, err := client.Generate(t.Context(), testRequest())
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("错误为 %v，期望 %v", err, tt.wantErr)
			}
			if body.closes != 1 {
				t.Errorf("响应关闭次数为 %d，期望 1", body.closes)
			}
		})
	}
}

func TestGeneratePreservesCloseErrors(t *testing.T) {
	readErr := errors.New("测试读取失败")
	closeErr := errors.New("测试关闭失败")
	tests := []struct {
		name       string
		reader     io.Reader
		statusCode int
		wantErr    error
		wantText   string
		wantAPIErr bool
	}{
		{name: "正常响应", reader: strings.NewReader(`{"choices":[{"index":0,"message":{"content":"ok"},"finish_reason":"stop"}]}`), statusCode: http.StatusOK, wantText: "ok"},
		{name: "无效响应", reader: strings.NewReader(`{"choices":[]}`), statusCode: http.StatusOK, wantErr: model.ErrInvalidResponse},
		{name: "读取失败", reader: errorReader{err: readErr}, statusCode: http.StatusOK, wantErr: readErr},
		{name: "HTTP错误", reader: strings.NewReader(`{"error":{"message":"测试服务异常","type":"server_error"}}`), statusCode: http.StatusBadGateway, wantAPIErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := &trackedBody{Reader: tt.reader, closeErr: closeErr}
			client := newTestClient(t, func(req *http.Request) (*http.Response, error) {
				response := testResponse(req, "application/json", body)
				response.StatusCode = tt.statusCode
				return response, nil
			})
			result, err := client.Generate(t.Context(), testRequest())
			if !errors.Is(err, closeErr) || (tt.wantErr != nil && !errors.Is(err, tt.wantErr)) {
				t.Fatalf("原始或关闭错误丢失：%v", err)
			}
			if tt.wantAPIErr {
				if _, ok := errors.AsType[*openai.Error](err); !ok {
					t.Fatal("供应商错误链丢失")
				}
			}
			assertCallError(t, err, tt.statusCode, testRequestID)
			if result.Text != tt.wantText {
				t.Errorf("响应文本为 %q，期望 %q", result.Text, tt.wantText)
			}
			if body.closes != 1 {
				t.Errorf("响应关闭次数为 %d，期望 1", body.closes)
			}
		})
	}
}

func TestGenerateRedirectFailureDoesNotCloseResponseTwice(t *testing.T) {
	redirectErr := errors.New("测试拒绝重定向")
	body := &trackedBody{Reader: strings.NewReader("")}
	client, err := New(Config{BaseURL: "https://model.invalid/v1", APIKey: "test-api-key"}, &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return redirectErr },
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			response := testResponse(req, "application/json", body)
			response.StatusCode = http.StatusFound
			response.Header.Set("Location", "/redirected")
			return response, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Generate(t.Context(), testRequest()); !errors.Is(err, redirectErr) {
		t.Fatalf("重定向错误链丢失：%v", err)
	}
	if body.closes != 1 {
		t.Errorf("响应关闭次数为 %d，期望 1", body.closes)
	}
}
