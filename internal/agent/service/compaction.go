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
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/tokens"
)

// ExecuteCompaction 不创建或更新回答正文；连接取消和应用退出仍经统一调度器收敛。
func (e *Engine) ExecuteCompaction(ctx context.Context, lease *executor.Lease, ex conversation.Execution, resolved agent.Resolved) {
	err := e.event(ex, "run.started", "", "", ex.Run)
	e.beginContextUsage(ex, resolved)
	defer e.endContextUsage(ex)
	if err == nil {
		err = e.event(ex, "step.created", ex.Step.ID, "", ex.Step)
	}
	if err == nil && (ex.Run.SettingsSnapshot == nil || !ex.Run.SettingsSnapshot.Valid() || ex.Run.SettingsSnapshot.Compaction == nil) {
		err = agent.ErrInvalid
	}
	if err == nil {
		traces, _, readErr := e.prepareHistory(ctx, &ex, resolved, true)
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
		e.endContextUsage(ex)
		_ = e.emit(agent.Event{Type: "run.failed", ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, Data: ex.Run, StatePersisted: new(false)})
		return
	}
	// 用量刷新先于终态通知，连接可在终态后排空并结束。
	e.endContextUsage(ex)
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
		plan, err := e.compressionPlan(ctx, *ex, resolved, traces, ex.Run.SettingsSnapshot.Compaction.RetainTurns)
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

// compactOld 压缩较早完整轮次；没有可用轮次时由调用方继续尝试步骤级压缩。
func (e *Engine) compactOld(ctx context.Context, ex *conversation.Execution, resolved agent.Resolved, traces map[string][]model.Message, retain int) (bool, error) {
	plan, err := e.compressionPlan(ctx, *ex, resolved, traces, retain)
	if err != nil {
		return false, err
	}
	if len(plan.Sources) == 0 {
		return false, nil
	}
	id, err := uuid.NewV7()
	if err != nil {
		return false, err
	}
	now := time.Now().UnixMilli()
	input, err := json.Marshal(map[string]any{"purpose": "context_compaction", "trigger": map[string]string{"type": "automatic", "reason": "context_capacity"}})
	if err != nil {
		return false, err
	}
	step, err := e.repository.CreateStep(ctx, agent.Step{ID: id.String(), ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, Type: "context_compression", Name: "压缩历史上下文", Status: agent.Pending, Input: input, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return false, operationStorageError(ctx, err)
	}
	if err := e.event(*ex, "step.created", step.ID, "", step); err != nil {
		return false, err
	}
	version, err := e.compress(ctx, *ex, resolved, step, plan)
	if err != nil {
		return false, err
	}
	ex.Context = version
	kept := make([]conversation.Message, 0, len(ex.History))
	for _, row := range ex.History {
		if row.Sequence > plan.Through {
			kept = append(kept, row)
		}
	}
	ex.History = kept
	return true, nil
}

func summaryInput(version *conversation.ContextVersion, messages []model.Message) []model.Message {
	input := append([]model.Message{{Role: model.RoleSystem, Content: compaction.Prompt}}, contextMessages(version, messages)...)
	return append(input, model.Message{Role: model.RoleUser, Content: "请压缩上述历史，只返回规定的 JSON 摘要。"})
}

func (e *Engine) compressionPlan(ctx context.Context, ex conversation.Execution, resolved agent.Resolved, traces map[string][]model.Message, retain int) (compaction.Plan, error) {
	cfg := ex.Run.SettingsSnapshot.Compaction
	budget, budgetErr := cfg.InputBudget(resolved.ContextWindow, resolved.MaxOutputTokens)
	var fitsErr error
	plan, err := compaction.BuildWithin(ex.History, traces, retain, func(messages []model.Message) bool {
		if budgetErr != nil {
			fitsErr = model.ErrTokenBudget
			return false
		}
		if tokens.Estimate(summaryInput(ex.Context, messages), nil, cfg.ImageTokenEstimate) > budget {
			fitsErr = model.ErrTokenLimit
			return false
		}
		fitsErr = inputCapacity(ctx, ex, resolved, summaryInput(ex.Context, messages), nil, nil)
		return fitsErr == nil
	})
	if err != nil && fitsErr != nil {
		return compaction.Plan{}, fitsErr
	}
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
	if err := inputCapacity(ctx, ex, resolved, input, nil, nil); err != nil {
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
