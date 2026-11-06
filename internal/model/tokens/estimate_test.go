package tokens_test

import (
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/tokens"
)

func TestEstimateIncludesToolsAndUsesEffectiveContent(t *testing.T) {
	plain := []model.Message{{Role: model.RoleUser, Content: "hello"}}
	base := tokens.Estimate(plain, nil, 8192)
	tool := model.ToolDefinition{Name: "edit_file", Description: strings.Repeat("修改代码，保留约束。", 100), Parameters: []byte(`{"type":"object","properties":{"content":{"type":"string"}}}`)}
	if tokens.Estimate(plain, []model.ToolDefinition{tool}, 8192) <= base+500 {
		t.Fatal("工具定义没有计入预算")
	}
	blocked := []model.Message{{Role: model.RoleUser, Content: strings.Repeat("ignored", 1000), Blocks: []model.Block{{Type: "text", Text: "hello"}}}}
	if tokens.Estimate(blocked, nil, 8192) != base {
		t.Fatal("重复计算被 Blocks 替代的 Content")
	}
	image := []model.Message{{Role: model.RoleUser, Blocks: []model.Block{{Type: "image", Path: "/short.png"}}}}
	before := tokens.Estimate(image, nil, 8192)
	image[0].Blocks[0].Path = "/" + strings.Repeat("long", 1000) + ".png"
	if tokens.Estimate(image, nil, 8192) != before || before < 8192 {
		t.Fatal("图片按路径字节数计算 Token")
	}
	trace := []model.Message{{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: "call", Name: "edit_file", Arguments: []byte(`{"content":"代码"}`)}}}, {Role: model.RoleTool, ToolCallID: "call", Content: strings.Repeat("result", 1000)}}
	if tokens.Estimate(trace, nil, 8192) < 2000 {
		t.Fatal("工具调用或结果未计入预算")
	}
	for _, text := range []string{"中文约束必须保留", "func main() { println(123) }", "emoji: 👩‍💻"} {
		if tokens.Text(text) <= 0 {
			t.Fatal("非英文或代码未计入预算")
		}
	}
}

func TestActualInputReferenceOnlyAppliesToIdenticalPrefix(t *testing.T) {
	base := []model.Message{{Role: model.RoleUser, Content: "hello"}}
	actual := int64(1000)
	var meter tokens.Meter
	meter.Observe(base, nil, &actual)
	next := append(append([]model.Message{}, base...), model.Message{Role: model.RoleTool, Content: strings.Repeat("tool output ", 100)})
	result := meter.Estimate(next, nil, 8192)
	if result.Tokens <= actual || result.ReferenceInputTokens != actual {
		t.Fatal("没有在实际输入用量基础上计算新增工具内容")
	}
	next[0].Content = "历史已压缩"
	if got := meter.Estimate(next, nil, 8192); got.ReferenceInputTokens != 0 || got.Method != tokens.Method {
		t.Fatal("压缩后复用了旧 usage")
	}
	if got := meter.Estimate(base, []model.ToolDefinition{{Name: "changed"}}, 8192); got.ReferenceInputTokens != 0 {
		t.Fatal("工具定义变化后复用旧 usage")
	}
	low := int64(1)
	meter.Observe(base, nil, &low)
	if meter.Estimate(base, nil, 8192).Tokens != tokens.Estimate(base, nil, 8192) {
		t.Fatal("较小 usage 覆盖了本地估算")
	}
	meter.Observe(base, nil, nil)
	if meter.Estimate(base, nil, 8192).ReferenceInputTokens != 0 {
		t.Fatal("缺失 usage 时推测了实际用量")
	}
}

func TestReasoningCountsTowardInputBudget(t *testing.T) {
	messages := []model.Message{{Role: model.RoleAssistant, Content: "回答"}}
	before := tokens.Estimate(messages, nil, 0)
	messages[0].Reasoning = new(strings.Repeat("推理", 100))
	if tokens.Estimate(messages, nil, 0) != before+tokens.Text(*messages[0].Reasoning) {
		t.Fatal("推理未计入输入预算")
	}
	var meter tokens.Meter
	actual := int64(1000)
	meter.Observe(messages, nil, &actual)
	messages[0].Reasoning = new("不同推理")
	if meter.Estimate(messages, nil, 0).ReferenceInputTokens != 0 {
		t.Fatal("推理变化后复用了旧输入用量")
	}
}
