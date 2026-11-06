package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/settings"
)

func TestManualSummaryRequiresBudgetWhenAutomaticCompactionDisabled(t *testing.T) {
	cfg := settings.DefaultCompaction()
	cfg.Enabled, cfg.TokenBudget = false, 100
	ex := conversation.Execution{Run: agent.Run{SettingsSnapshot: &settings.Snapshot{Compaction: &settings.CompactionSnapshot{Compaction: cfg}}}}
	messages := []model.Message{{Role: model.RoleUser, Content: strings.Repeat("必须保留约束", 100)}}
	if err := inputCapacity(t.Context(), ex, agent.Resolved{}, messages, nil, nil); err != nil {
		t.Fatalf("关闭自动压缩仍强制普通请求 Token 预算: %v", err)
	}
	if err := summaryCapacity(t.Context(), ex, agent.Resolved{}, messages); !errors.Is(err, model.ErrTokenLimit) {
		t.Fatalf("手动步骤摘要绕过 Token 预算: %v", err)
	}
	ex.Run.SettingsSnapshot.Compaction.TokenBudget = 0
	if err := summaryCapacity(t.Context(), ex, agent.Resolved{}, messages); !errors.Is(err, model.ErrTokenBudget) {
		t.Fatalf("手动步骤摘要猜测未知容量: %v", err)
	}
}
