package execution_test

import (
	"encoding/json"
	"testing"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/stdio/execution"
)

func TestToolOutputRetainsExecutionIdentity(t *testing.T) {
	value := agent.ToolOutput{Sequence: 1, Stream: "stderr", Encoding: "base64", Data: "AP8="}
	err := execution.EmitPersistent(func(method string, params any) error {
		if method != "tool.output" {
			t.Fatal(method)
		}
		raw, err := json.Marshal(params)
		if err != nil {
			return err
		}
		var received struct {
			ConversationID string           `json:"conversation_id"`
			RunID          string           `json:"run_id"`
			StepID         string           `json:"step_id"`
			Output         agent.ToolOutput `json:"output"`
		}
		if err := json.Unmarshal(raw, &received); err != nil {
			return err
		}
		if received.ConversationID != "c" || received.RunID != "r" || received.StepID != "s" || received.Output != value {
			t.Fatalf("工具分片身份或数据丢失: %s", raw)
		}
		return nil
	}, agent.Event{Type: "tool.output", ConversationID: "c", RunID: "r", StepID: "s", Data: value})
	if err != nil {
		t.Fatal(err)
	}
}
