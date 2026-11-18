package chatcompletions

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/model"
)

func TestCaptureOriginalUsageAndProviderFields(t *testing.T) {
	raw := `{"id":"response-1","provider_extra":{"trace":"kept"},"usage":{"prompt_tokens":4,"completion_tokens":2,"vendor_detail":{"cached":1}},"choices":[{"index":0,"message":{"role":"assistant","content":"完成"},"finish_reason":"stop"}]}`
	body := &trackedBody{Reader: strings.NewReader(raw)}
	client := newTestClient(t, func(req *http.Request) (*http.Response, error) {
		return testResponse(req, "application/json", body), nil
	})
	result, err := client.Generate(t.Context(), testRequest())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(result.Output, []byte(`"provider_extra":{"trace":"kept"}`)) || !bytes.Contains(result.Usage, []byte(`"vendor_detail":{"cached":1}`)) || bytes.Contains(result.Output, []byte("test-api-key")) {
		t.Fatalf("原始结果或 usage 未保留: %s %s", result.Output, result.Usage)
	}
	if body.closes != 1 {
		t.Fatal("响应未释放")
	}
}

func TestInputTokensOnlyUsesActualPromptTokens(t *testing.T) {
	for _, raw := range []string{`{}`, `{"total_tokens":7}`, `{"prompt_tokens":null}`, `{"prompt_tokens":-1}`, `{"prompt_tokens":1.5}`, `{"prompt_tokens":"100"}`} {
		if got := inputTokens([]byte(raw)); got != nil {
			t.Fatalf("推测或接受了无效输入用量: %s %d", raw, *got)
		}
	}
	if got := inputTokens([]byte(`{"prompt_tokens":12,"total_tokens":999}`)); got == nil || *got != 12 {
		t.Fatal("实际 prompt_tokens 丢失或使用了累计值")
	}
}

func TestCaptureStreamCompletionAndInterruption(t *testing.T) {
	for _, complete := range []bool{true, false} {
		t.Run(map[bool]string{true: "正常", false: "异常中断"}[complete], func(t *testing.T) {
			chunks := []string{`{"id":"r1","vendor_meta":"kept","choices":[{"index":0,"delta":{"role":"assistant","content":"你","reasoning_content":"a","vendor_part":"x"},"logprobs":{"content":[{"token":"你"}]},"finish_reason":null}]}`, `{"id":"r1","choices":[{"index":0,"delta":{"content":"好","reasoning_content":"b","vendor_part":"y"},"logprobs":{"content":[{"token":"好"}]},"finish_reason":null}]}`}
			if complete {
				chunks = append(chunks, strings.Replace(stopChunk, `"index":0,`, `"index":0,"logprobs":null,`, 1), `{"choices":[],"usage":{"total_tokens":7,"vendor":{"value":9}}}`, "[DONE]")
			}
			body := &trackedBody{Reader: strings.NewReader(sse(chunks...))}
			client := newStreamClient(t, body)
			req := testRequest()
			req.CaptureResult = true
			var result *model.Response
			completed := false
			var reasoning strings.Builder
			err := client.Stream(t.Context(), req, func(event model.Event) error {
				if event.Type == model.EventReasoningDelta {
					reasoning.WriteString(event.Text)
				}
				if event.Type == model.EventResult {
					if body.closes != 1 {
						t.Error("结果交付前响应未关闭")
					}
					result = event.Result
				}
				completed = completed || event.Type == model.EventCompleted
				return nil
			})
			if (err == nil) != complete || completed != complete || result == nil || result.Text != "你好" {
				t.Fatalf("流式结果不正确: %v %+v", err, result)
			}
			if reasoning.String() != "ab" || result.Reasoning == nil || *result.Reasoning != "ab" {
				t.Fatalf("统一推理字段或增量丢失: %+v", result)
			}
			if !bytes.Contains(result.Output, []byte(`"reasoning_content":"ab"`)) || !bytes.Contains(result.Output, []byte(`"vendor_meta":"kept"`)) {
				t.Fatalf("组装丢失返回字段: %s", result.Output)
			}
			if !bytes.Contains(result.Output, []byte(`"vendor_part":["x","y"]`)) || !bytes.Contains(result.Output, []byte(`"content":[{"token":"你"},{"token":"好"}]`)) {
				t.Fatalf("扩展片段或 logprobs 丢失: %s", result.Output)
			}
			if complete {
				if !bytes.Contains(result.Usage, []byte(`"vendor":{"value":9}`)) {
					t.Fatalf("usage 被改写: %s", result.Usage)
				}
			} else if result.Usage != nil {
				t.Fatalf("缺少 usage 时伪造用量: %s", result.Usage)
			}
			if body.closes != 1 {
				t.Fatal("响应未释放")
			}
		})
	}
}

func TestLocalImageInputAndFailures(t *testing.T) {
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "input.png")
	if err := os.WriteFile(path, imageBytes.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	called := 0
	client := newTestClient(t, func(req *http.Request) (*http.Response, error) {
		called++
		var wire struct {
			Messages []struct {
				Content []struct {
					Type     string `json:"type"`
					ImageURL struct {
						URL string `json:"url"`
					} `json:"image_url"`
				} `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(req.Body).Decode(&wire); err != nil {
			t.Fatal(err)
		}
		if len(wire.Messages) != 1 || len(wire.Messages[0].Content) != 2 || wire.Messages[0].Content[1].ImageURL.URL != "data:image/png;base64,"+base64.StdEncoding.EncodeToString(imageBytes.Bytes()) {
			t.Fatalf("图片路径未转换为媒体内容: %+v", wire)
		}
		return testResponse(req, "application/json", &trackedBody{Reader: strings.NewReader(`{"choices":[{"index":0,"message":{"role":"assistant","content":"图片已收到"},"finish_reason":"stop"}]}`)}), nil
	})
	req := model.Request{Model: "vision", AllowImageInput: true, Messages: []model.Message{{Role: model.RoleUser, Blocks: []model.Block{{Type: "text", Text: "分析"}, {Type: "image", Path: path}}}}}
	if _, err := client.Generate(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	req.Messages[0].Blocks[1].Path = filepath.Join(t.TempDir(), "missing.png")
	if _, err := client.Generate(t.Context(), req); !errors.Is(err, model.ErrImageRead) {
		t.Fatalf("缺失图片错误不明确: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.Generate(ctx, req); !errors.Is(err, context.Canceled) {
		t.Fatalf("读取图片忽略取消: %v", err)
	}
	if called != 1 {
		t.Fatalf("错误输入仍调用远端: %d", called)
	}
}
