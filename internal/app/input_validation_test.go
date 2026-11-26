package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaoyu98628/callum/internal/conversation"
)

func TestPersistentAPIsRejectNonRFCUUIDv7(t *testing.T) {
	rt, _ := runtimeFixture(t, 1)
	peer := sharedPeer(t, rt)
	for _, method := range []string{"provider.get", "model.get", "workspace.get", "conversation.get", "run.get", "step.get", "approval.get"} {
		frame := peer.call(t, method, map[string]string{"id": "01901234-1234-7234-0234-123456789abc"})
		if frame.Error == nil || frame.Error.Code != -32602 {
			t.Errorf("%s 接受了非标准 UUIDv7: %+v", method, frame)
		}
	}
}

func TestInvalidAttachmentsReturnControlledInputError(t *testing.T) {
	rt, _ := runtimeFixture(t, 1)
	modelID := directModel(t, rt, "https://example.test")
	peer := sharedPeer(t, rt)
	directory := t.TempDir()
	oversized := filepath.Join(directory, "oversized.bin")
	file, err := os.Create(oversized)
	if err != nil {
		t.Fatal(err)
	}
	if err := errors.Join(file.Truncate((16<<20)+1), file.Close()); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(directory, "missing.txt"), directory, oversized} {
		frame := peer.call(t, "conversation.send", map[string]any{"model_id": modelID, "content": []map[string]string{{"type": "file", "path": path, "name": "input.bin"}}})
		if frame.Error == nil || frame.Error.Code != 1013 {
			t.Fatalf("附件输入错误被当作服务内部失败: %+v", frame)
		}
		var data struct {
			Kind string `json:"kind"`
		}
		if json.Unmarshal(frame.Error.Data, &data) != nil || data.Kind != "attachment_unavailable" {
			t.Fatalf("附件错误缺少分类: %+v", frame.Error)
		}
	}
	items, err := rt.Conversations().List(t.Context(), conversation.Filter{})
	if err != nil || len(items) != 0 {
		t.Fatalf("无效附件受理了会话: %+v %v", items, err)
	}
}
