package app

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/xiaoyu98628/callum/internal/config"
)

func TestImageInputUsesSelectedModelType(t *testing.T) {
	for _, test := range []struct {
		name         string
		capabilities map[string]bool
		types        []string
		allowed      bool
	}{
		{"capability_without_types", map[string]bool{"image_input": true}, nil, false},
		{"capability_with_text_type", map[string]bool{"image_input": true}, []string{"text"}, false},
		{"image_type_without_capability", nil, []string{"image"}, true},
		{"image_type_with_false_capability", map[string]bool{"image_input": false}, []string{"image"}, true},
		{"image_type_with_capability", map[string]bool{"image_input": true}, []string{"text", "image"}, true},
		{"other_types_without_image", nil, []string{"text", "audio", "video"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				var request struct {
					Tools    []json.RawMessage `json:"tools"`
					Messages []struct {
						Role    string          `json:"role"`
						Content json.RawMessage `json:"content"`
					} `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if len(request.Tools) != 9 || len(request.Messages) != 2 || request.Messages[0].Role != "system" || request.Messages[1].Role != "user" {
					t.Error("图片请求缺少固定提示或用户消息")
					w.WriteHeader(400)
					return
				}
				var parts []struct {
					Type     string `json:"type"`
					ImageURL struct {
						URL string `json:"url"`
					} `json:"image_url"`
				}
				if err := json.Unmarshal(request.Messages[1].Content, &parts); err != nil || len(parts) != 2 || !strings.HasPrefix(parts[1].ImageURL.URL, "data:image/png;base64,") {
					t.Error("图片转换或默认工具定义丢失")
				}
				w.Header().Set("Content-Type", "application/json")
				if _, err := io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"图片已收到"},"finish_reason":"stop"}]}`); err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(upstream.Close)
			cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
			if err != nil {
				t.Fatal(err)
			}
			cfg.MaxConcurrentRuns = 1
			p := startApp(t, cfg, io.Discard)
			pid := resultString(t, p.call(t, "provider.create", map[string]any{"name": "图片厂商", "protocol": "chatcompletions", "base_url": upstream.URL + "/v1"}), "id")
			mid := resultString(t, p.call(t, "model.create", map[string]any{"provider_id": pid, "display_name": "图片模型", "remote_model_id": "vision", "enabled": true, "context_window": 1000000, "capabilities": test.capabilities, "model_type": test.types}), "id")
			var data bytes.Buffer
			if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "input.png")
			if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			frame := p.call(t, "conversation.send", map[string]any{"model_id": mid, "stream": false, "content": []map[string]string{{"type": "text", "text": "描述图片"}, {"type": "image", "path": path}}})
			if test.allowed {
				requireOK(t, frame)
				p.event(t, "run.completed")
				if requests.Load() != 1 {
					t.Fatal("选中图片类型后未完成调用")
				}
			} else if frame.Error == nil || frame.Error.Code != 1005 || requests.Load() != 0 {
				t.Fatalf("未选中图片类型仍被受理或调用失败: %+v", frame)
			}
			p.stop(t)
		})
	}
}
