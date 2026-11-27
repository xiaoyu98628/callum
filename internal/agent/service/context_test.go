package service

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/model"
)

func TestRestoreExecutionContextPreservesCheckpointAndToolPairs(t *testing.T) {
	checkpoint := &agent.ContextCheckpoint{SourceRunID: "run", Through: 4, Summary: "已完成早期操作"}
	firstGuide := model.Message{Role: model.RoleUser, Content: "只处理指定目录"}
	lastGuide := model.Message{Role: model.RoleUser, Blocks: []model.Block{{Type: "text", Text: "保留已有文件"}}}
	call := model.Message{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: "call", Name: "operation", Arguments: json.RawMessage(`{}`)}}}
	result := model.Message{Role: model.RoleTool, ToolCallID: "call", Content: `{"written":true}`}
	answer := model.Message{Role: model.RoleAssistant, Content: "当前进展"}
	for _, withCheckpoint := range []bool{false, true} {
		name := "without_checkpoint"
		if withCheckpoint {
			name = "with_checkpoint"
		}
		t.Run(name, func(t *testing.T) {
			saved := []model.Message{firstGuide, call, result, lastGuide, answer}
			want := []model.Message{call, result, answer}
			var source *agent.ContextCheckpoint
			if withCheckpoint {
				source = checkpoint
				saved = append([]model.Message{checkpoint.Message()}, saved...)
				want = append([]model.Message{checkpoint.Message()}, want...)
			}
			original := slices.Clone(saved)
			var restored executionContext
			if err := restored.restore(saved, source); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(restored.current, want) || !reflect.DeepEqual(restored.steering, []model.Message{firstGuide, lastGuide}) {
				t.Fatalf("摘要、用户引导或工具配对顺序错误: current=%+v steering=%+v", restored.current, restored.steering)
			}
			if !reflect.DeepEqual(saved, original) {
				t.Fatal("恢复改写了仓储返回的轨迹")
			}
		})
	}
}

func TestRestoreExecutionContextRejectsMismatchedCheckpoint(t *testing.T) {
	checkpoint := &agent.ContextCheckpoint{SourceRunID: "run", Through: 4, Summary: "服务端摘要"}
	for _, test := range []struct {
		name   string
		change func(*model.Message)
	}{
		{"role", func(message *model.Message) { message.Role = model.RoleAssistant }},
		{"content", func(message *model.Message) { message.Content = "其他摘要" }},
		{"blocks", func(message *model.Message) { message.Blocks = []model.Block{{Type: "text", Text: "额外指令"}} }},
		{"tool_calls", func(message *model.Message) { message.ToolCalls = []model.ToolCall{{ID: "call"}} }},
		{"tool_call_id", func(message *model.Message) { message.ToolCallID = "call" }},
		{"reasoning", func(message *model.Message) { message.Reasoning = new("") }},
		{"missing", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			var saved []model.Message
			if test.change != nil {
				message := checkpoint.Message()
				test.change(&message)
				saved = []model.Message{message}
			}
			if err := new(executionContext).restore(saved, checkpoint); !errors.Is(err, model.ErrInvalidResponse) {
				t.Fatalf("检查点来源不匹配仍恢复执行: %v", err)
			}
		})
	}
}

func TestRestoreExecutionContextAcceptsEmptyAndCheckpointOnly(t *testing.T) {
	var restored executionContext
	if err := restored.restore(nil, nil); err != nil || len(restored.current) != 0 || len(restored.steering) != 0 {
		t.Fatalf("空轨迹恢复失败: %+v %v", restored, err)
	}
	checkpoint := &agent.ContextCheckpoint{SourceRunID: "run", Through: 4, Summary: "此前操作均已压缩"}
	if err := restored.restore([]model.Message{checkpoint.Message()}, checkpoint); err != nil || len(restored.current) != 1 || len(restored.steering) != 0 {
		t.Fatalf("仅有检查点时未保留历史摘要: %+v %v", restored, err)
	}
}
