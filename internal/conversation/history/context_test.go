package history_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/conversation/history"
	"github.com/xiaoyu98628/callum/internal/model"
)

func TestContextRestoresFactsAfterTheirUserMessage(t *testing.T) {
	messages := []conversation.Message{
		{RunID: "failed", Role: "user", Content: []model.Block{{Type: "text", Text: "执行操作"}}},
		{RunID: "failed", Role: "assistant", Status: agent.Failed, Content: []model.Block{{Type: "text", Text: "未完成回答"}}},
		{RunID: "completed", Role: "user", Content: []model.Block{{Type: "text", Text: "查询事实"}}},
		{RunID: "completed", Role: "assistant", Status: agent.Completed, Content: []model.Block{{Type: "text", Text: "最终回答"}}},
		{RunID: "next", Role: "user", Content: []model.Block{{Type: "text", Text: "继续"}}},
	}
	facts := []model.Message{{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: "call", Name: "operation"}}}, {Role: model.RoleTool, ToolCallID: "call", Content: `{"operation":{"version":1,"outcome":"applied"}}`}}
	out, err := history.Context(messages, map[string][]model.Message{"failed": facts, "unrelated": {{Role: model.RoleAssistant, Content: "其他会话"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 6 || !reflect.DeepEqual(out[1:3], facts) || out[4].Blocks[0].Text != "最终回答" || out[5].Blocks[0].Text != "继续" {
		t.Fatalf("上下文错序或丢失: %+v", out)
	}
	for _, message := range out {
		for _, block := range message.Blocks {
			if block.Text == "未完成回答" {
				t.Fatal("恢复了失败回答")
			}
		}
	}
	// 成功执行的完整轨迹替代回答消息，避免重复回答。
	out, err = history.Context(messages[2:4], map[string][]model.Message{"completed": {{Role: model.RoleAssistant, Content: "最终回答"}}})
	if err != nil || len(out) != 2 || out[1].Content != "最终回答" {
		t.Fatalf("成功回答重复: %+v %v", out, err)
	}
}

func TestSkillContextKeepsTaskTitleAndSeparateReference(t *testing.T) {
	content := []model.Block{{Type: "skill", Name: "Review", Path: "/skills/review/SKILL.md"}, {Type: "text", Text: "核对正文"}}
	before := append([]model.Block(nil), content...)
	if got := history.Title(content); got != "核对正文" {
		t.Fatalf("标题包含技能引用: %q", got)
	}
	blocks := history.ModelContent(content)
	if len(blocks) != 2 || blocks[0].Text != "$Review 核对正文" || !strings.Contains(blocks[1].Text, content[0].Path) {
		t.Fatalf("模型缺少引用或正文: %+v", blocks)
	}
	if !reflect.DeepEqual(content, before) {
		t.Fatal("组装模型上下文修改了持久化内容")
	}
}

func TestRestoredFactsStillRespectContextLimits(t *testing.T) {
	for _, facts := range [][]model.Message{
		make([]model.Message, 256),
		{{Role: model.RoleTool, Content: strings.Repeat("x", history.MaxBytes)}},
	} {
		_, err := history.Context([]conversation.Message{{Role: "user", RunID: "run"}}, map[string][]model.Message{"run": facts})
		if !errors.Is(err, model.ErrInputLimit) {
			t.Fatalf("恢复轨迹绕过输入限制: %v", err)
		}
	}
}
