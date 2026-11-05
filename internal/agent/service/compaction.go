package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/executor"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/conversation/compaction"
	"github.com/xiaoyu98628/callum/internal/conversation/history"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/tokens"
)

// ExecuteCompaction 不创建或更新回答正文；连接取消和应用退出仍经统一调度器收敛。
func (e *Engine) ExecuteCompaction(ctx context.Context, lease *executor.Lease, ex conversation.Execution, resolved agent.Resolved) {
	err := e.event(ex, "run.started", "", "", ex.Run)
	if err == nil {
		err = e.event(ex, "step.created", ex.Step.ID, "", ex.Step)
	}
	if err == nil && (ex.Run.SettingsSnapshot == nil || !ex.Run.SettingsSnapshot.Valid() || ex.Run.SettingsSnapshot.Compaction == nil) {
		err = agent.ErrInvalid
	}
	if err == nil {
		floor := int64(0)
		if ex.Context != nil {
			floor = ex.Context.ThroughSequence
		}
		traces, readErr := e.repository.ContextTranscript(ctx, ex.Conversation.ID, floor)
		err = readErr
		if err == nil {
			err = e.manualCompaction(ctx, &ex, resolved, traces)
		}
	}
	if final := lease.Finishing(); final != nil {
		err = errors.Join(final, err)
	}
	ex.Run.Status = agent.Completed
	if err != nil {
		status, kind, message := failure(err)
		ex.Run.Status, ex.Run.ErrorKind, ex.Run.ErrorMessage = status, &kind, &message
	}
	now := time.Now().UnixMilli()
	ex.Run.UpdatedAt, ex.Run.FinishedAt = now, &now
	cleanup, cancel := cleanupContext(ctx)
	defer cancel()
	if finishErr := e.repository.Finish(cleanup, ex.Run); finishErr != nil {
		e.logger.Error("保存压缩执行终态失败", "module", "compaction", "run_id", ex.Run.ID)
		ex.Run.Status, ex.Run.ErrorKind, ex.Run.ErrorMessage = agent.Failed, new("internal"), new("保存执行状态失败")
		_ = e.emit(agent.Event{Type: "run.failed", ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, Data: ex.Run, StatePersisted: new(false)})
		return
	}
	_ = e.event(ex, "run."+ex.Run.Status, "", "", ex.Run)
}

// manualCompaction 按完整轮次持续推进，每批生成可追溯版本；资源有界，不限制调用批次。
func (e *Engine) manualCompaction(ctx context.Context, ex *conversation.Execution, resolved agent.Resolved, traces map[string][]model.Message) error {
	step := ex.Step
	first := true
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		plan, err := compressionPlan(*ex, resolved, traces, ex.Run.SettingsSnapshot.Compaction.RetainTurns)
		if err != nil {
			return err
		}
		if len(plan.Sources) == 0 && !first {
			return nil
		}
		if !first {
			id, err := uuid.NewV7()
			if err != nil {
				return err
			}
			now := time.Now().UnixMilli()
			step, err = e.repository.CreateStep(ctx, agent.Step{ID: id.String(), ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, Type: "context_compression", Name: "继续压缩历史上下文", Status: agent.Pending, Input: ex.Step.Input, CreatedAt: now, UpdatedAt: now})
			if err != nil {
				return operationStorageError(ctx, err)
			}
			if err := e.event(*ex, "step.created", step.ID, "", step); err != nil {
				return err
			}
		}
		version, err := e.compress(ctx, *ex, resolved, step, plan)
		if err != nil {
			return err
		}
		if len(plan.Sources) == 0 {
			return nil
		}
		first = false
		ex.Context = version
		kept := make([]conversation.Message, 0, len(ex.History))
		for _, row := range ex.History {
			if row.Sequence > plan.Through {
				kept = append(kept, row)
			}
		}
		ex.History = kept
	}
}

func contextMessages(version *conversation.ContextVersion, messages []model.Message) []model.Message {
	if version == nil {
		return messages
	}
	return append([]model.Message{compaction.SummaryMessage(*version)}, messages...)
}

// automaticContext 只压缩较早完整轮次，当前运行新增的工具链始终保留原文。
func (e *Engine) automaticContext(ctx context.Context, ex *conversation.Execution, resolved agent.Resolved, traces map[string][]model.Message, messages, fixed []model.Message, definitions []model.ToolDefinition, meter *tokens.Meter) ([]model.Message, error) {
	snapshot := ex.Run.SettingsSnapshot
	if snapshot == nil || snapshot.Compaction == nil || !snapshot.Compaction.Enabled {
		return messages, nil
	}
	cfg := snapshot.Compaction
	budget, err := cfg.InputBudget(resolved.ContextWindow, resolved.MaxOutputTokens)
	if err != nil {
		return nil, model.ErrTokenBudget
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		effective := append(fixed, contextMessages(ex.Context, messages)...)
		estimate := meter.Estimate(effective, definitions, cfg.ImageTokenEstimate)
		if estimate.Tokens <= budget {
			return messages, nil
		}
		plan, err := compressionPlan(*ex, resolved, traces, cfg.RetainTurns+1)
		if err != nil {
			return nil, err
		}
		if len(plan.Sources) == 0 {
			return nil, model.ErrTokenLimit
		}
		id, err := uuid.NewV7()
		if err != nil {
			return nil, err
		}
		now := time.Now().UnixMilli()
		input, err := json.Marshal(map[string]any{"purpose": "context_compaction", "trigger": map[string]any{"type": "automatic", "reason": "token_budget", "token_budget": budget, "estimate": estimate}})
		if err != nil {
			return nil, err
		}
		step, err := e.repository.CreateStep(ctx, agent.Step{ID: id.String(), ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, Type: "context_compression", Name: "压缩历史上下文",
			Status: agent.Pending, Input: input, CreatedAt: now, UpdatedAt: now})
		if err != nil {
			return nil, operationStorageError(ctx, err)
		}
		if err := e.event(*ex, "step.created", step.ID, "", step); err != nil {
			return nil, err
		}
		version, err := e.compress(ctx, *ex, resolved, step, plan)
		if err != nil {
			return nil, err
		}
		ex.Context = version
		kept := make([]conversation.Message, 0, len(ex.History))
		for _, row := range ex.History {
			if row.Sequence > plan.Through {
				kept = append(kept, row)
			}
		}
		ex.History = kept
		messages = messages[len(plan.Messages):]
	}
}

func summaryInput(version *conversation.ContextVersion, messages []model.Message) []model.Message {
	input := append([]model.Message{{Role: model.RoleSystem, Content: compaction.Prompt}}, contextMessages(version, messages)...)
	return append(input, model.Message{Role: model.RoleUser, Content: "请压缩上述历史，只返回规定的 JSON 摘要。"})
}

func compressionPlan(ex conversation.Execution, resolved agent.Resolved, traces map[string][]model.Message, retain int) (compaction.Plan, error) {
	cfg := ex.Run.SettingsSnapshot.Compaction
	budget, budgetErr := cfg.InputBudget(resolved.ContextWindow, resolved.MaxOutputTokens)
	plan, err := compaction.BuildWithin(ex.History, traces, retain, func(messages []model.Message) bool {
		return budgetErr == nil && tokens.Estimate(summaryInput(ex.Context, messages), nil, cfg.ImageTokenEstimate) <= budget
	})
	if budgetErr != nil && err != nil {
		return compaction.Plan{}, model.ErrTokenBudget
	}
	// 每次成功必须消费新的源范围；无进展直接失败，不通过固定次数掩盖循环问题。
	if err == nil && len(plan.Sources) > 0 {
		floor := int64(0)
		if ex.Context != nil {
			floor = ex.Context.ThroughSequence
		}
		if len(plan.Messages) == 0 || plan.Through <= floor || len(ex.History) == 0 || plan.Through < ex.History[0].Sequence {
			return compaction.Plan{}, compaction.ErrNoReduction
		}
	}
	return plan, err
}

func (e *Engine) compress(ctx context.Context, ex conversation.Execution, resolved agent.Resolved, step agent.Step, plan compaction.Plan) (version *conversation.ContextVersion, retErr error) {
	var metadata map[string]any
	if err := json.Unmarshal(step.Input, &metadata); err != nil {
		return nil, err
	}
	metadata["through_sequence"], metadata["source_message_count"] = plan.Through, len(plan.Sources)
	if ex.Context != nil {
		metadata["base_context_version_id"] = ex.Context.ID
	}
	if snapshot := ex.Run.SettingsSnapshot; snapshot != nil && snapshot.Compaction != nil {
		metadata["compaction_settings_revision"] = snapshot.Compaction.Revision
	}
	inputMetadata, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	step.Input = inputMetadata
	now := time.Now().UnixMilli()
	step.Status, step.StartedAt, step.UpdatedAt = agent.Running, &now, now
	if err := e.repository.SaveStep(ctx, step); err != nil {
		return nil, operationStorageError(ctx, err)
	}
	if err := e.event(ex, "step.updated", step.ID, "", step); err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		cleanup, cancel := cleanupContext(ctx)
		defer cancel()
		if version != nil {
			retErr = errors.Join(retErr, e.conversations.FailContext(cleanup, version.ID))
		}
		step.Status = agent.Completed
		if retErr != nil {
			status, kind, message := failure(retErr)
			step.Status, step.ErrorKind, step.ErrorMessage = status, &kind, &message
		}
		now := time.Now().UnixMilli()
		step.UpdatedAt, step.FinishedAt = now, &now
		if err := e.repository.SaveStep(cleanup, step); err != nil {
			retErr = errors.Join(retErr, agent.ErrStorage, err)
		} else {
			_ = e.event(ex, "step.updated", step.ID, "", step)
		}
	}()
	if len(plan.Sources) == 0 {
		step.Output = json.RawMessage(`{"compressed":false,"reason":"no_earlier_complete_turns"}`)
		return ex.Context, nil
	}
	source := contextMessages(ex.Context, plan.Messages)
	input := summaryInput(ex.Context, plan.Messages)
	cfg := ex.Run.SettingsSnapshot.Compaction
	budget, err := cfg.InputBudget(resolved.ContextWindow, resolved.MaxOutputTokens)
	if err != nil {
		return nil, model.ErrTokenBudget
	}
	if tokens.Estimate(input, nil, cfg.ImageTokenEstimate) > budget {
		return nil, model.ErrTokenLimit
	}
	if err := history.Check(input); err != nil {
		return nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	value := conversation.ContextVersion{ID: id.String(), ConversationID: ex.Conversation.ID, GeneratedByStepID: step.ID, Sources: plan.Sources, ThroughSequence: plan.Through}
	if ex.Context != nil {
		value.BaseVersionID = &ex.Context.ID
	}
	value, err = e.conversations.CreateContext(ctx, value)
	if err != nil {
		return nil, operationStorageError(ctx, err)
	}
	version = &value
	result, err := e.summaryModel(ctx, ex, resolved, step, input, value.ID)
	if err != nil {
		return version, err
	}
	if len(result.ToolCalls) != 0 {
		return version, model.ErrInvalidResponse
	}
	summary, err := compaction.Validate(result.Text)
	if err != nil {
		return version, err
	}
	value.Summary = summary
	before, err := json.Marshal(source)
	if err != nil {
		return version, err
	}
	after, err := json.Marshal([]model.Message{compaction.SummaryMessage(value)})
	if err != nil {
		return version, err
	}
	beforeTokens := tokens.Estimate(source, nil, cfg.ImageTokenEstimate)
	afterTokens := tokens.Estimate([]model.Message{compaction.SummaryMessage(value)}, nil, cfg.ImageTokenEstimate)
	if afterTokens >= beforeTokens {
		return version, compaction.ErrNoReduction
	}
	value.Stats, err = json.Marshal(map[string]any{"before_messages": len(source), "after_messages": 1, "before_bytes": len(before), "after_bytes": len(after), "estimated_before_tokens": beforeTokens, "estimated_after_tokens": afterTokens, "estimation_method": tokens.Method})
	if err != nil {
		return version, err
	}
	if err := ctx.Err(); err != nil {
		return version, err
	}
	if err := e.conversations.ActivateContext(ctx, value); err != nil {
		return version, operationStorageError(ctx, err)
	}
	committed = true
	now = time.Now().UnixMilli()
	value.Status, value.ActivatedAt, value.UpdatedAt = "active", &now, now
	version = &value
	step.Status, step.FinishedAt, step.UpdatedAt = agent.Completed, &now, now
	step.Output, err = json.Marshal(map[string]any{"compressed": true, "context_version_id": value.ID, "size_stats": value.Stats})
	if err != nil {
		return version, err
	}
	return version, e.event(ex, "step.updated", step.ID, "", step)
}
