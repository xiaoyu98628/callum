package chatcompletions

import (
	"bytes"
	"context"
	"encoding/base64"
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
