package compaction_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/conversation/compaction"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/tokens"
)

func TestPlanKeepsCompleteToolPairsAndCurrentTurn(t *testing.T) {
	rows := []conversation.Message{
		{ID: "u1", RunID: "r1", Sequence: 1, Role: "user", Content: []model.Block{{Type: "text", Text: "旧问题"}}},
		{ID: "a1", RunID: "r1", Sequence: 2, Role: "assistant", Status: "completed"},
		{ID: "u2", RunID: "r2", Sequence: 3, Role: "user", Content: []model.Block{{Type: "text", Text: "保留的问题"}}},
		{ID: "a2", RunID: "r2", Sequence: 4, Role: "assistant", Status: "completed"},
		{ID: "u3", RunID: "r3", Sequence: 5, Role: "user", Content: []model.Block{{Type: "text", Text: "当前问题"}}},
	}
	trace := []model.Message{{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: "c", Name: "read_file", Arguments: []byte(`{}`)}}}, {Role: model.RoleTool, ToolCallID: "c", Content: "事实"}}
	plan, err := compaction.Build(rows, map[string][]model.Message{"r1": trace}, 2)
	if err != nil || plan.Through != 2 || len(plan.Sources) != 2 || len(plan.Messages) != 3 {
		t.Fatalf("边界错误: %+v %v", plan, err)
	}
	if len(plan.Messages[1].ToolCalls) != 1 || plan.Messages[2].ToolCallID != "c" {
		t.Fatal("工具配对丢失")
	}
	hash := plan.Sources[0].Fingerprint
	trace[1].Content = "改变的事实"
	changed, err := compaction.Build(rows, map[string][]model.Message{"r1": trace}, 2)
	if err != nil || changed.Sources[0].Fingerprint == hash {
		t.Fatal("轨迹变化未反映到指纹")
	}
}

func TestPlanBatchesWithinTokenBudgetWithoutSplittingTurns(t *testing.T) {
	rows := []conversation.Message{
		{ID: "u1", RunID: "r1", Sequence: 1, Role: "user", Content: []model.Block{{Type: "text", Text: strings.Repeat("old ", 100)}}},
		{ID: "a1", RunID: "r1", Sequence: 2, Role: "assistant", Status: "completed"},
		{ID: "u2", RunID: "r2", Sequence: 3, Role: "user", Content: []model.Block{{Type: "text", Text: strings.Repeat("large ", 500)}}},
		{ID: "a2", RunID: "r2", Sequence: 4, Role: "assistant", Status: "completed"},
		{ID: "u3", RunID: "r3", Sequence: 5, Role: "user", Content: []model.Block{{Type: "text", Text: "current"}}},
	}
	fits := func(messages []model.Message) bool { return tokens.Estimate(messages, nil, 8192) <= 400 }
	plan, err := compaction.BuildWithin(rows, nil, 1, fits)
	if err != nil || plan.Through != 2 || len(plan.Sources) != 2 {
		t.Fatalf("未按完整旧轮次缩小摘要源批次: %+v %v", plan, err)
	}
	if _, err := compaction.BuildWithin(rows, nil, 1, func([]model.Message) bool { return false }); !errors.Is(err, model.ErrTokenLimit) {
		t.Fatalf("不可容纳完整源轮次时未明确失败: %v", err)
	}
}

func TestSummaryRequiresBoundedStructuredData(t *testing.T) {
	valid := `{"goals":["保留目标"],"constraints":["禁止重复执行"],"decisions":[],"facts":["结果未知"],"open_questions":[]}`
	if _, err := compaction.Validate(valid); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{
		"随意摘要", "```json\n" + valid + "\n```", valid + " {}", strings.Repeat("x", compaction.MaxSummaryBytes+1),
		`{"goals":[],"constraints":[],"decisions":[],"facts":[],"open_questions":[]}`,
		`{"goals":["任务"],"constraints":[],"decisions":[],"facts":null,"open_questions":[]}`,
		`{"goals":["任务"],"constraints":[],"decisions":[],"facts":[],"facts":[],"open_questions":[]}`,
	} {
		if _, err := compaction.Validate(invalid); err == nil {
			t.Fatalf("接受非法摘要: %s", invalid[:min(len(invalid), 100)])
		}
	}
}
