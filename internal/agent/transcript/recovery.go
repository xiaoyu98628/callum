package transcript

import (
	"encoding/json"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/tool"
)

// RecoverOutput 根据已保存的执行边界恢复工具事实，保留已知结果且不重新执行操作。
// 返回独立的 JSON 输出；调用方负责将其与执行终态在同一事务中保存。
func RecoverOutput(row agent.Step) (json.RawMessage, error) {
	if row.ToolCallID == nil || *row.ToolCallID == "" {
		return nil, model.ErrInvalidResponse
	}
	var input struct {
		RequestHash string `json:"request_hash"`
	}
	if err := json.Unmarshal(row.Input, &input); err != nil {
		return nil, err
	}
	fields := map[string]json.RawMessage{}
	if len(row.Output) > 0 && json.Unmarshal(row.Output, &fields) != nil || fields == nil {
		return nil, model.ErrInvalidResponse
	}
	facts := tool.OutcomeRecord{Version: tool.OperationVersion, Outcome: tool.OutcomeNotApplied, RequestHash: input.RequestHash, Recovered: true}
	if raw, ok := fields["operation"]; ok {
		if json.Unmarshal(raw, &facts) != nil || !facts.Valid() || facts.RequestHash != input.RequestHash {
			return nil, model.ErrInvalidResponse
		}
		facts.Recovered = true
	} else if row.StartedAt != nil {
		var definition tool.Definition
		if json.Unmarshal(row.Definition, &definition) != nil {
			return nil, model.ErrInvalidResponse
		}
		facts.Outcome = tool.UnconfirmedOutcome(definition.Effects)
	}
	var err error
	fields["operation"], err = json.Marshal(facts)
	if err != nil {
		return nil, err
	}
	if _, ok := fields["result"]; !ok {
		fields["result"] = json.RawMessage(`null`)
	}
	if _, ok := fields["error"]; !ok {
		kind, message := "result_unconfirmed", "操作返回结果未完整保存，请核实已记录的操作事实"
		if facts.Outcome == tool.OutcomeUnknown {
			kind, message = "outcome_unknown", "操作可能已经生效，请核实后再重试"
		} else if facts.Outcome == tool.OutcomeNotApplied {
			kind, message = "not_executed", "该操作尚未执行"
		} else if facts.Outcome == tool.OutcomeNone {
			message = "工具返回结果未完整保存，该工具不产生副作用"
		}
		fields["error"], err = json.Marshal(map[string]string{"kind": kind, "message": message})
		if err != nil {
			return nil, err
		}
	}
	delete(fields, "context_message")
	body, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	fields["context_message"], err = json.Marshal(model.Message{Role: model.RoleTool, Content: string(body), ToolCallID: *row.ToolCallID})
	if err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}
