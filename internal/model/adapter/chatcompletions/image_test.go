package chatcompletions

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/model"
)

func TestImagePreparationReusedByPreflightAndSend(t *testing.T) {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	wireImage := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data.Bytes())
	reads := 0
	client := newTestClient(t, func(req *http.Request) (*http.Response, error) {
		wire, err := io.ReadAll(req.Body)
		if err != nil || !bytes.Contains(wire, []byte(wireImage)) {
			t.Fatalf("实际发送的数据不一致: %v", err)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{"choices":[{"index":0,"message":{"role":"assistant","content":"已收到"},"finish_reason":"stop"}]}`))}, nil
	})
	client.readAttachment = func(context.Context, string) ([]byte, string, error) {
		reads++
		return data.Bytes(), "image/png", nil
	}
	req := model.Request{Model: "vision", AllowImageInput: true, Messages: []model.Message{{
		Role: model.RoleUser, Blocks: []model.Block{{Type: "image", AttachmentRef: "managed"}},
	}}}
	if err := client.ValidateInput(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Generate(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if reads != 1 {
		t.Fatalf("预检与发送重复读取图片: %d", reads)
	}
}

func TestToolImagesFollowCompleteToolBatchAndSurviveTextModelSwitch(t *testing.T) {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	client := newTestClient(t, nil)
	client.readAttachment = func(context.Context, string) ([]byte, string, error) { return data.Bytes(), "image/png", nil }
	request := model.Request{Model: "vision", AllowImageInput: true, Messages: []model.Message{
		{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: "first", Name: "view_image", Arguments: json.RawMessage(`{}`)}, {ID: "second", Name: "read_file", Arguments: json.RawMessage(`{}`)}}},
		{Role: model.RoleTool, ToolCallID: "first", Blocks: []model.Block{{Type: "text", Text: "first tool result"}, {Type: "image", AttachmentRef: "managed"}}},
		{Role: model.RoleTool, ToolCallID: "second", Content: "second tool result"},
		{Role: model.RoleAssistant, Content: "review completed"},
	}}
	for _, vision := range []bool{true, false} {
		request.AllowImageInput = vision
		parameters, err := client.buildRequest(t.Context(), request)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(parameters)
		if err != nil {
			t.Fatal(err)
		}
		var wire struct {
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(encoded, &wire); err != nil {
			t.Fatal(err)
		}
		if vision {
			if len(wire.Messages) != 5 || wire.Messages[1].Role != "tool" || wire.Messages[2].Role != "tool" || wire.Messages[3].Role != "user" || !bytes.Contains(wire.Messages[3].Content, []byte("data:image/png;base64,")) || bytes.Contains(wire.Messages[3].Content, []byte("first tool result")) {
				t.Fatalf("视觉内容破坏工具配对或重复正文: %s", encoded)
			}
		} else if len(wire.Messages) != 4 || bytes.Contains(encoded, []byte("image_url")) || !bytes.Contains(encoded, []byte("未提供给当前文字模型")) {
			t.Fatalf("文字模型切换没有明确省略历史工具图片: %s", encoded)
		}
	}
}
