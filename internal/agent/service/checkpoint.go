package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/conversation/compaction"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/tokens"
)

func checkpointInput(base *agent.ContextCheckpoint, messages []model.Message) []model.Message {
	input := []model.Message{{Role: model.RoleSystem, Content: compaction.Prompt}}
	if base != nil {
		input = append(input, base.Message())
	}
	input = append(input, messages...)
	return append(input, model.Message{Role: model.RoleUser, Content: "请压缩上述已完成过程，只返回规定的 JSON 摘要。保留约束、已发生操作及未解决事项，不推断未执行操作的结果。"})
}

// compactRun 每次只消费完整模型及工具批次。before 保护正在执行的批次；keepLast 保留最近完整批次。
// 新摘要通过结构、容量、缩减和来源校验后才生效；原始步骤及实际 usage 不变。
func (e *Engine) compactRun(ctx context.Context, ex conversation.Execution, resolved agent.Resolved, runID string, before int64, keepLast bool) (*agent.ContextCheckpoint, int, error) {
	base, err := e.repository.LatestCheckpoint(ctx, ex.Conversation.ID, runID)
	if err != nil {
		return nil, 0, operationStorageError(ctx, err)
	}
	cursor := int64(0)
	if base != nil {
		cursor = base.Through
	}
	var sources []agent.BatchSource
	var messages []model.Message
	for {
		batch, err := e.repository.ContextBatch(ctx, ex.Conversation.ID, runID, cursor)
		if err != nil {
			if capacityError(err) && len(sources) > 0 {
				break
			}
			return nil, 0, err
		}
		if batch == nil || (before > 0 && batch.Through >= before) {
			break
		}
		if keepLast {
			next, err := e.repository.ContextBatch(ctx, ex.Conversation.ID, runID, batch.Through)
			if err != nil && !capacityError(err) {
				return nil, 0, err
			}
			if err == nil && next == nil {
				break
			}
		}
		if len(batch.Messages) == 0 {
			break
		}
		candidate := append(append([]model.Message(nil), messages...), batch.Messages...)
		raw, err := json.Marshal(candidate)
		if err != nil {
			return nil, 0, err
		}
		if len(candidate) > 248 || len(raw) > 7<<20 {
			if len(sources) == 0 {
				return nil, 0, model.ErrInputLimit
			}
			break
		}
		if err := summaryCapacity(ctx, ex, resolved, checkpointInput(base, candidate)); err != nil {
			if !capacityError(err) || len(sources) == 0 {
				return nil, 0, err
			}
			break
		}
		messages = candidate
		sources = append(sources, batch.BatchSource)
		cursor = batch.Through
	}
	if len(sources) == 0 {
		return nil, 0, nil
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, 0, err
	}
	checkpoint := agent.ContextCheckpoint{Version: 1, ID: id.String(), ConversationID: ex.Conversation.ID, SourceRunID: runID, Through: cursor, Sources: sources}
	if base != nil {
		checkpoint.BaseID, checkpoint.After = base.ID, base.Through
	}
	input, err := json.Marshal(map[string]any{"purpose": "context_compaction", "trigger": map[string]string{"type": "automatic", "reason": "context_capacity"}, "source_run_id": runID, "after_step_sequence": checkpoint.After, "through_step_sequence": cursor, "source_batches": sources})
	if err != nil {
		return nil, 0, err
	}
	now := time.Now().UnixMilli()
	step, err := e.repository.CreateStep(ctx, agent.Step{ID: checkpoint.ID, ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, Type: "context_compression", Name: "压缩已完成过程", Status: agent.Running, Input: input, CreatedAt: now, StartedAt: &now, UpdatedAt: now})
	if err != nil {
		return nil, 0, operationStorageError(ctx, err)
	}
	if err := e.event(ex, "step.created", step.ID, "", step); err != nil {
		return nil, 0, err
	}
	value, err := e.generateCheckpoint(ctx, ex, resolved, step, checkpoint, base, messages)
	if err != nil {
		return nil, 0, err
	}
	return value, len(messages), nil
}

func (e *Engine) generateCheckpoint(ctx context.Context, ex conversation.Execution, resolved agent.Resolved, step agent.Step, value agent.ContextCheckpoint, base *agent.ContextCheckpoint, messages []model.Message) (out *agent.ContextCheckpoint, retErr error) {
	committed := false
	defer func() {
		if committed {
			return
		}
		cleanup, cancel := cleanupContext(ctx)
		defer cancel()
		status, kind, message := failure(retErr)
		now := time.Now().UnixMilli()
		step.Status, step.ErrorKind, step.ErrorMessage, step.FinishedAt, step.UpdatedAt = status, &kind, &message, &now, now
		if err := e.repository.SaveStep(cleanup, step); err != nil {
			retErr = errors.Join(retErr, agent.ErrStorage, err)
		} else {
			_ = e.event(ex, "step.updated", step.ID, "", step)
		}
	}()
	result, err := e.summaryModel(ctx, ex, resolved, step, checkpointInput(base, messages), value.ID)
	if err != nil {
		return nil, err
	}
	if len(result.ToolCalls) != 0 {
		return nil, model.ErrInvalidResponse
	}
	value.Summary, err = compaction.Validate(result.Text)
	if err != nil {
		return nil, err
	}
	source := messages
	if base != nil {
		source = append([]model.Message{base.Message()}, source...)
	}
	target := []model.Message{value.Message()}
	prior, err := json.Marshal(source)
	if err != nil {
		return nil, err
	}
	after, err := json.Marshal(target)
	if err != nil {
		return nil, err
	}
	cfg := ex.Run.SettingsSnapshot.Compaction
	beforeTokens, afterTokens := tokens.Estimate(source, nil, cfg.ImageTokenEstimate), tokens.Estimate(target, nil, cfg.ImageTokenEstimate)
	if len(after) >= len(prior) || afterTokens >= beforeTokens || len(target) > len(source) {
		return nil, compaction.ErrNoReduction
	}
	if err := summaryCapacity(ctx, ex, resolved, target); err != nil {
		return nil, err
	}
	value.Stats, err = json.Marshal(map[string]any{"before_messages": len(source), "after_messages": 1, "before_bytes": len(prior), "after_bytes": len(after), "estimated_before_tokens": beforeTokens, "estimated_after_tokens": afterTokens, "estimation_method": tokens.Method})
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := e.repository.SaveCheckpoint(ctx, value); err != nil {
		return nil, operationStorageError(ctx, err)
	}
	committed = true
	now := time.Now().UnixMilli()
	step.Status, step.FinishedAt, step.UpdatedAt = agent.Completed, &now, now
	step.Output, err = json.Marshal(map[string]any{"compressed": true, "checkpoint": value})
	if err != nil {
		return &value, err
	}
	return &value, e.event(ex, "step.updated", step.ID, "", step)
}
