package agent

import (
	"context"
	"encoding/json"

	domain "github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/tool"
)

// OperationFacts 从当前分支提取每个固定操作的最新待核实状态，摘要无权覆盖这些记录。
func (r *Repository) OperationFacts(ctx context.Context, id string) ([]domain.OperationFact, error) {
	var invalid int64
	if err := r.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM conversation_run_steps s
		JOIN conversation_messages m ON m.run_id=s.run_id AND m.role='user'
		WHERE s.conversation_id=? AND m.conversation_id=? AND s.type='tool_call' AND json_extract(s.input,'$.operation_version')=1
		AND (COALESCE(json_extract(s.output,'$.operation.version'),0)<>1
		OR COALESCE(json_extract(s.output,'$.operation.outcome'),'') NOT IN ('none','not_applied','applied','partially_applied','unknown')
 OR COALESCE(json_extract(s.output,'$.operation.request_hash'),'')<>COALESCE(json_extract(s.input,'$.request_hash'),'')
 OR length(COALESCE(json_extract(s.output,'$.operation.request_hash'),''))>128)`, id, id).Scan(&invalid).Error; err != nil {
		return nil, err
	}
	if invalid != 0 {
		return nil, model.ErrInvalidResponse
	}
	var rows []struct {
		StepID, RunID, Tool, RequestHash, Operation string
	}
	// 先按操作哈希取最新结果，再过滤未确认状态。成功调用不会让集合随历史增长。
	if err := r.db.WithContext(ctx).Raw(`WITH facts AS (
		SELECT s.id AS step_id,s.run_id,s.name AS tool,
			json_extract(s.output,'$.operation.request_hash') AS request_hash,
			json_extract(s.output,'$.operation') AS operation,
			json_extract(s.output,'$.operation.outcome') AS outcome,
			COALESCE(json_type(s.output,'$.error'),'null') AS error_type,
			ROW_NUMBER() OVER (PARTITION BY json_extract(s.output,'$.operation.request_hash') ORDER BY m.sequence DESC,s.sequence DESC) AS rn
		FROM conversation_run_steps s
		JOIN conversation_messages m ON m.run_id=s.run_id AND m.role='user'
		WHERE s.conversation_id=? AND m.conversation_id=? AND s.type='tool_call' AND json_extract(s.output,'$.operation.version')=1
			AND COALESCE(json_extract(s.output,'$.operation.request_hash'),'')<>''
			AND json_extract(s.output,'$.operation.outcome')<>'not_applied'
	)
	SELECT step_id,run_id,tool,request_hash,operation FROM facts WHERE rn=1
		AND (outcome IN ('unknown','partially_applied') OR (outcome='applied' AND error_type<>'null'))
	ORDER BY step_id LIMIT 4097`, id, id).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) > 4096 {
		return nil, model.ErrInputLimit
	}
	out := make([]domain.OperationFact, 0, len(rows))
	for _, row := range rows {
		var value tool.OutcomeRecord
		if json.Unmarshal([]byte(row.Operation), &value) != nil || !value.Valid() || value.RequestHash != row.RequestHash {
			return nil, model.ErrInvalidResponse
		}
		out = append(out, domain.OperationFact{StepID: row.StepID, RunID: row.RunID, Tool: row.Tool, RequestHash: row.RequestHash, Outcome: string(value.Outcome)})
	}
	return out, nil
}
