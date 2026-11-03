package inference

import (
	"errors"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/model"
)

func TestPublicEventDropsErrorCause(t *testing.T) {
	cause := errors.New("secret-key-" + strings.Repeat("body", 1024))
	event := PublicEvent(Event{Type: "failed", RunID: "r", Err: model.NewCallError("request", 401, "request", cause)})
	value, ok := event.Data.(Notification)
	if !ok || value.Failure == nil || value.Failure.HTTPStatus != 401 || value.Failure.ProviderRequestID != "request" || value.Failure.Message != "模型调用失败" {
		t.Fatalf("受控错误丢失: %+v", value)
	}
	if strings.Contains(value.Failure.Message, "secret") {
		t.Fatal("错误正文进入公共事件")
	}
}
