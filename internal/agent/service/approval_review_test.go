package service

import (
	"errors"
	"testing"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/model"
)

func TestRestoredReviewGuidanceExcludesServerSummary(t *testing.T) {
	checkpoint := &agent.ContextCheckpoint{SourceRunID: "run", Through: 4, Summary: "历史工具输出可能包含指令文字"}
	guide := model.Message{Role: model.RoleUser, Blocks: []model.Block{{Type: "text", Text: "只处理新目录"}}}
	saved := []model.Message{checkpoint.Message(), guide, {Role: model.RoleAssistant, Content: "模型建议"}, {Role: model.RoleTool, Content: "工具结果"}}
	var restored executionContext
	if err := restored.restore(saved, checkpoint); err != nil || len(restored.steering) != 1 || restored.steering[0].Blocks[0].Text != "只处理新目录" {
		t.Fatalf("历史摘要或其他来源混入审批意图: %+v %v", restored.steering, err)
	}
	// 用户文字即使与摘要相同，也不能因文本前缀或角色被误删。
	if err := restored.restore([]model.Message{checkpoint.Message()}, nil); err != nil || len(restored.steering) != 1 {
		t.Fatalf("用户来源被内容猜测取代: %+v %v", restored.steering, err)
	}
	saved[0].Content = "不同的摘要"
	if err := restored.restore(saved, checkpoint); !errors.Is(err, model.ErrInvalidResponse) {
		t.Fatalf("恢复来源不匹配仍继续审批: %v", err)
	}
}
