package service

import (
	"context"
	"errors"
	"testing"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/model"
)

func TestCorrectionRejectsJoinedLifecycleFailures(t *testing.T) {
	bad := &model.ToolArgumentsError{Index: 2, Fingerprint: "same"}
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded, agent.ErrStorage, errors.New("close response failed")} {
		if _, ok := soleArgumentError(errors.Join(model.NewCallError("decode response", 200, "request", bad), cause)); ok {
			t.Fatalf("纠正覆盖了其他失败: %v", cause)
		}
	}
	if got, ok := soleArgumentError(errors.Join(model.NewCallError("decode response", 200, "request", bad))); !ok || got != bad {
		t.Fatal("没有识别单独的工具参数错误")
	}
}

func TestRejectedArgumentsRoundRequiresNoEffects(t *testing.T) {
	calls := []model.ToolCall{{ID: "one", Name: "run_command", Arguments: []byte(`{"command":"[]"}`)}}
	result := []model.Message{{Role: model.RoleTool, ToolCallID: "one", Content: `{"operation":{"version":1,"outcome":"not_applied"},"error":{"kind":"invalid_arguments"}}`}}
	fingerprint := rejectedArgumentsRound(calls, result)
	if fingerprint == "" {
		t.Fatal("没有识别执行前拒绝")
	}
	calls[0].ID = "new-id"
	result[0].ToolCallID = "new-id"
	if rejectedArgumentsRound(calls, result) != fingerprint {
		t.Fatal("调用标识改变掩盖了重复坏参数")
	}
	for _, body := range []string{
		`{"operation":{"version":1,"outcome":"unknown"},"error":{"kind":"invalid_arguments"}}`,
		`{"operation":{"version":1,"outcome":"applied"},"error":{"kind":"invalid_arguments"}}`,
		`{"operation":{"version":1,"outcome":"not_applied"},"error":{"kind":"permission_denied"}}`,
	} {
		result[0].Content = body
		if rejectedArgumentsRound(calls, result) != "" {
			t.Fatal("把已执行或权限错误作为参数纠正")
		}
	}
}
