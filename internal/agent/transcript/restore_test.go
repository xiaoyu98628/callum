package transcript_test

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/transcript"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/tool"
)

func encode(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func partialTrace(t *testing.T) []transcript.Entry {
	t.Helper()
	return []transcript.Entry{
		{StepID: "model", Message: encode(t, model.Message{Role: model.RoleAssistant, Content: "未完成正文", ToolCalls: []model.ToolCall{{ID: "a", Name: "write"}, {ID: "b", Name: "write"}}})},
		{StepID: "tool", ParentStepID: new("model"), ToolCallID: new("a"), Message: encode(t, model.Message{Role: model.RoleTool, ToolCallID: "a", Content: "已记录结果"}), Operation: encode(t, tool.OutcomeRecord{Version: 1, Outcome: tool.OutcomeUnknown})},
	}
}

func TestRestoreKeepsFactsAndCompletesInterruptedToolPairs(t *testing.T) {
	for _, status := range []string{agent.Failed, agent.Cancelled, agent.Interrupted} {
		t.Run(status, func(t *testing.T) {
			entries := partialTrace(t)
			before := string(entries[0].Message)
			messages, err := transcript.Restore(status, entries)
			if err != nil || len(messages) != 3 || messages[0].Content != "" || len(messages[0].ToolCalls) != 2 || messages[1].Content != "已记录结果" || messages[1].ToolCallID != "a" || messages[2].ToolCallID != "b" {
				t.Fatalf("恢复丢失事实或调用配对: %+v %v", messages, err)
			}
			var missing struct {
				Operation tool.OutcomeRecord    `json:"operation"`
				Error     struct{ Kind string } `json:"error"`
			}
			if err := json.Unmarshal([]byte(messages[2].Content), &missing); err != nil || missing.Operation.Outcome != tool.OutcomeNotApplied || !missing.Operation.Recovered || missing.Error.Kind != "not_executed" {
				t.Fatalf("尚未执行的调用被推测为已执行: %+v %v", missing, err)
			}
			if string(entries[0].Message) != before {
				t.Fatal("恢复改写了来源记录")
			}
		})
	}
}

func TestRestoreRejectsCorruptOrUnpairedFacts(t *testing.T) {
	for _, name := range []string{"duplicate", "missing_parent", "wrong_call", "invalid_message", "invalid_operation", "orphan_effect"} {
		t.Run(name, func(t *testing.T) {
			entries := partialTrace(t)
			switch name {
			case "duplicate":
				entries = append(entries, entries[1])
			case "missing_parent":
				entries[1].ParentStepID = nil
			case "wrong_call":
				entries[1].ToolCallID = new("other")
			case "invalid_message":
				entries[0].Message = json.RawMessage(`{`)
			case "invalid_operation":
				entries[1].Operation = json.RawMessage(`{"version":1,"outcome":"invalid"}`)
			case "orphan_effect":
				entries[1].ParentStepID = new("unrelated")
			}
			if _, err := transcript.Restore(agent.Interrupted, entries); !errors.Is(err, model.ErrInvalidResponse) {
				t.Fatalf("损坏轨迹未被拒绝: %v", err)
			}
		})
	}
	entries := partialTrace(t)
	if _, err := transcript.Restore(agent.Completed, entries); !errors.Is(err, model.ErrInvalidResponse) {
		t.Fatalf("成功轨迹接受不完整配对: %v", err)
	}
	entries = append(slices.Clone(entries), transcript.Entry{StepID: "tool-b", ParentStepID: new("model"), ToolCallID: new("b"), Message: encode(t, model.Message{Role: model.RoleTool, ToolCallID: "b", Content: "第二个结果"})})
	entries = append(entries, transcript.Entry{StepID: "final", Message: encode(t, model.Message{Role: model.RoleAssistant, Content: "完整回答"})})
	messages, err := transcript.Restore(agent.Completed, entries)
	if err != nil || len(messages) != 4 || messages[0].Content != "未完成正文" || messages[3].Content != "完整回答" {
		t.Fatalf("完整轨迹被错误过滤: %+v %v", messages, err)
	}
}
