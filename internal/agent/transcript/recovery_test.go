package transcript_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/transcript"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/tool"
)

func TestRecoverOutputPreservesKnownFactsAndMarksExecutionBoundary(t *testing.T) {
	for _, tt := range []struct {
		name    string
		started bool
		effects []tool.Effect
		known   tool.Outcome
		want    tool.Outcome
	}{
		{"not_started", false, []tool.Effect{tool.EffectWrite}, "", tool.OutcomeNotApplied},
		{"started_write", true, []tool.Effect{tool.EffectWrite}, "", tool.OutcomeUnknown},
		{"started_read", true, []tool.Effect{tool.EffectRead}, "", tool.OutcomeNone},
		{"applied", true, []tool.Effect{tool.EffectWrite}, tool.OutcomeApplied, tool.OutcomeApplied},
		{"partial", true, []tool.Effect{tool.EffectWrite}, tool.OutcomePartial, tool.OutcomePartial},
	} {
		t.Run(tt.name, func(t *testing.T) {
			step := agent.Step{ToolCallID: new("call"), Input: json.RawMessage(`{"request_hash":"fixed"}`), Definition: encode(t, tool.Definition{Effects: tt.effects})}
			if tt.started {
				step.StartedAt = new(int64(1))
			}
			if tt.known != "" {
				step.Output = encode(t, map[string]any{"operation": tool.OutcomeRecord{Version: 1, RequestHash: "fixed", Outcome: tt.known}, "result": map[string]bool{"saved": true}, "error": map[string]string{"kind": "original"}, "context_message": model.Message{Content: "过期副本"}})
			}
			before := string(step.Output)
			raw, err := transcript.RecoverOutput(step)
			if err != nil {
				t.Fatal(err)
			}
			var recovered struct {
				Operation tool.OutcomeRecord    `json:"operation"`
				Result    json.RawMessage       `json:"result"`
				Error     struct{ Kind string } `json:"error"`
				Message   model.Message         `json:"context_message"`
			}
			if err := json.Unmarshal(raw, &recovered); err != nil || recovered.Operation.Outcome != tt.want || !recovered.Operation.Recovered || recovered.Operation.RequestHash != "fixed" || recovered.Message.Role != model.RoleTool || recovered.Message.ToolCallID != "call" {
				t.Fatalf("恢复事实错误: %s %v", raw, err)
			}
			if tt.known != "" && (string(recovered.Result) != `{"saved":true}` || recovered.Error.Kind != "original") {
				t.Fatalf("已保存结果被覆盖: %s", raw)
			}
			var context map[string]json.RawMessage
			if err := json.Unmarshal([]byte(recovered.Message.Content), &context); err != nil || context["context_message"] != nil || string(context["operation"]) != string(encode(t, recovered.Operation)) {
				t.Fatalf("上下文没有独立保存相同事实: %+v %v", recovered.Message, err)
			}
			if string(step.Output) != before {
				t.Fatal("恢复改写了来源 JSON")
			}
		})
	}
}

func TestRecoverOutputRejectsMismatchedOrCorruptFacts(t *testing.T) {
	for _, output := range []string{`null`, `{`, `{"operation":{"version":1,"outcome":"applied","request_hash":"other"}}`, `{"operation":{"version":1,"outcome":"invalid","request_hash":"fixed"}}`} {
		_, err := transcript.RecoverOutput(agent.Step{ToolCallID: new("call"), Input: json.RawMessage(`{"request_hash":"fixed"}`), Output: json.RawMessage(output)})
		if !errors.Is(err, model.ErrInvalidResponse) {
			t.Fatalf("损坏事实未被拒绝: %s %v", output, err)
		}
	}
}
