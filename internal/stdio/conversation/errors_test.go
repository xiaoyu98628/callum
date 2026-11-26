package conversation

import (
	"context"
	"errors"
	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/model"
	"testing"
)

func TestAttachmentErrorsRemainActionable(t *testing.T) {
	for _, err := range []error{model.ErrAttachment, errors.Join(model.ErrAttachment, errors.New("private-path"))} {
		e := publicError(err)
		data, ok := e.Data.(map[string]any)
		if e.Code != 1013 || e.Message != model.ErrAttachment.Error() || !ok || data["kind"] != "attachment_unavailable" {
			t.Fatalf("附件错误未保留受控分类: %+v", e)
		}
	}
	e := publicError(errors.Join(agent.ErrStorage, model.ErrAttachment, context.Canceled))
	if e.Code != -32603 {
		t.Fatalf("持久化失败被附件分类掩盖: %+v", e)
	}
}
