package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"

	domain "github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	"github.com/xiaoyu98628/callum/internal/tool"
)

// 命令历史从实际程序、参数、工作目录和权限边界识别同一操作。
// 环境摘要及读取目录包含服务端新建的私有路径，仅用于本次固定请求，不参与历史匹配。
// 这里同时读取旧快照与当前快照，输入只由服务端生成。格式参数固定为 SQL 列或表达式。
const operationHistoryProjection = `CASE
	WHEN json_extract(%[1]s,'$.tool.name')='run_command'
		AND json_extract(%[1]s,'$.tool.source')='builtin'
		AND json_type(%[1]s,'$.arguments.command')='array'
		AND json_type(%[1]s,'$.details.executable_identity')='text'
		AND json_type(%[1]s,'$.details.cwd_identity')='text'
		AND json_type(%[1]s,'$.details.execution_boundary')='object'
	THEN 'command:' || json_array(
		json_remove(json_extract(%[1]s,'$.arguments.command'),'$[0]'),
		json_extract(%[1]s,'$.workspace.id','$.workspace.root_path',
			'$.details.executable','$.details.executable_identity',
			'$.details.executable_size','$.details.executable_modified_ns',
			'$.details.cwd','$.details.cwd_identity','$.details.execution_boundary',
			'$.details.sandbox_backend','$.details.writable_roots',
			'$.details.network_access','$.details.environment_profile',
			'$.details.environment_keys'))
	ELSE %[2]s END`

func operationHistoryKey(projection string) string {
	if !strings.HasPrefix(projection, "command:") {
		return projection
	}
	digest := sha256.Sum256([]byte(projection))
	return "command:" + hex.EncodeToString(digest[:])
}

// CommandHistoryKey 使用与旧记录查询相同的投影识别当前命令。实际审批仍使用完整请求哈希。
func (r *Repository) CommandHistoryKey(ctx context.Context, request []byte, hash string) (string, error) {
	var projection string
	query := "SELECT " + fmt.Sprintf(operationHistoryProjection, "request", "request_hash") +
		" FROM (SELECT ? AS request, ? AS request_hash)"
	if err := r.db.WithContext(ctx).Raw(query, string(request), hash).Scan(&projection).Error; err != nil {
		return "", err
	}
	return operationHistoryKey(projection), nil
}

// RejectedOperationKeys 恢复同一执行中已经拒绝的操作，继续任务不会重置这些决定。
func (r *Repository) RejectedOperationKeys(ctx context.Context, runID string) ([]string, error) {
	var keys []string
	projection := fmt.Sprintf(operationHistoryProjection, "request", "request_hash")
	if err := r.db.WithContext(ctx).Model(&schema.Approval{}).Where("run_id=? AND status=?", runID, domain.ApprovalRejected).
		Select("DISTINCT " + projection).Limit(4097).Scan(&keys).Error; err != nil {
		return nil, err
	}
	if len(keys) > 4096 {
		return nil, model.ErrInputLimit
	}
	for i := range keys {
		keys[i] = operationHistoryKey(keys[i])
	}
	return keys, nil
}

// OperationFacts 从当前分支提取每个固定操作的最新待核实状态，摘要无权覆盖这些记录。
func (r *Repository) OperationFacts(ctx context.Context, id string) ([]domain.OperationFact, error) {
	base := func() *gorm.DB {
		return r.db.WithContext(ctx).Model(&schema.Step{}).Table("conversation_run_steps AS s").
			Joins("JOIN conversation_messages m ON m.run_id=s.run_id AND m.role='user'").
			Where("s.conversation_id=? AND m.conversation_id=? AND s.type=?", id, id, "tool_call")
	}
	var invalid int64
	if err := base().Where("json_extract(s.input,'$.operation_version')=?", tool.OperationVersion).
		Where(`(COALESCE(json_extract(s.output,'$.operation.version'),0)<>1
		OR COALESCE(json_extract(s.output,'$.operation.outcome'),'') NOT IN ('none','not_applied','applied','partially_applied','unknown')
		OR COALESCE(json_extract(s.output,'$.operation.request_hash'),'')<>COALESCE(json_extract(s.input,'$.request_hash'),'')
		OR length(COALESCE(json_extract(s.output,'$.operation.request_hash'),''))>128)`).Count(&invalid).Error; err != nil {
		return nil, err
	}
	if invalid != 0 {
		return nil, model.ErrInvalidResponse
	}
	var rows []struct {
		StepID, RunID, Tool, RequestHash, Operation, HistoryProjection string
	}
	projection := fmt.Sprintf(operationHistoryProjection, "json_extract(s.input,'$.request')", "json_extract(s.output,'$.operation.request_hash')")
	// 命令跨临时目录使用同一历史匹配键，其他工具继续按完整请求哈希归组。
	// 先取最新的实际操作结果，确保已经确认成功的操作不再进入待核实集合。
	ranked := base().Select(`s.id AS step_id,s.run_id,s.name AS tool,
			json_extract(s.output,'$.operation.request_hash') AS request_hash,
			json_extract(s.output,'$.operation') AS operation,
			json_extract(s.output,'$.operation.outcome') AS outcome,
			COALESCE(json_type(s.output,'$.error'),'null') AS error_type,
			`+projection+` AS history_projection,
			ROW_NUMBER() OVER (PARTITION BY `+projection+` ORDER BY m.sequence DESC,s.sequence DESC) AS rn`).
		Where("json_extract(s.output,'$.operation.version')=?", tool.OperationVersion).
		Where("COALESCE(json_extract(s.output,'$.operation.request_hash'),'')<>''").
		Where("json_extract(s.output,'$.operation.outcome')<>?", tool.OutcomeNotApplied).
		// 命令以退出码 0 确认成功；非零退出仍记录原样，但在历史匹配时不能消解旧的待核实事实。
		Where(`NOT (s.name='run_command' AND COALESCE(json_extract(s.input,'$.request.tool.source'),'')='builtin'
			AND json_extract(s.output,'$.operation.outcome')=? AND COALESCE(json_type(s.output,'$.error'),'null')='null'
			AND COALESCE(json_extract(s.output,'$.result.exit_code'),-1)<>0)`, tool.OutcomeApplied)
	if err := r.db.WithContext(ctx).Table("(?) AS facts", ranked).
		Select("step_id,run_id,tool,request_hash,operation,history_projection").Where("rn=?", 1).
		Where("(outcome IN ? OR (outcome=? AND error_type<>?))", []tool.Outcome{tool.OutcomeUnknown, tool.OutcomePartial}, tool.OutcomeApplied, "null").
		Order("step_id").Limit(4097).Scan(&rows).Error; err != nil {
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
		out = append(out, domain.OperationFact{StepID: row.StepID, RunID: row.RunID, Tool: row.Tool, RequestHash: row.RequestHash, Outcome: string(value.Outcome), HistoryKey: operationHistoryKey(row.HistoryProjection)})
	}
	return out, nil
}
