package service

import (
	"context"
	"errors"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/conversation/compaction"
	"github.com/xiaoyu98628/callum/internal/conversation/history"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/tokens"
)

// executionContext 将当前用户要求和未完成批次独立保留，摘要只替换已验证的历史前缀。
type executionContext struct {
	traces  map[string][]model.Message
	past    []model.Message
	current []model.Message
}

func historyBoundary(ex conversation.Execution) (int64, int64) {
	floor := int64(0)
	if ex.Context != nil {
		floor = ex.Context.ThroughSequence
	}
	ceiling := ex.User.Sequence
	if ex.HistoryMore {
		ceiling = floor + 1
		for _, row := range ex.History {
			if row.ID != ex.User.ID {
				ceiling = max(ceiling, row.Sequence+1)
			}
		}
	}
	return floor, ceiling
}

func (e *Engine) reloadHistory(ctx context.Context, ex *conversation.Execution) error {
	rows, version, more, err := e.conversations.LoadContextHistory(ctx, ex.Conversation.ID, ex.User.Sequence)
	if err != nil {
		return operationStorageError(ctx, err)
	}
	if ex.User.ID != "" {
		rows = append(rows, ex.User)
	}
	ex.History, ex.Context, ex.HistoryMore = rows, version, more
	return nil
}

// compactHistorical 可在旧执行已超出整段恢复上限时，按有界完整批次修复其恢复上下文。
func (e *Engine) compactHistorical(ctx context.Context, ex conversation.Execution, resolved agent.Resolved) (bool, error) {
	seen := map[string]bool{}
	var capacityErr error
	for _, row := range ex.History {
		if row.Role != "user" || row.ID == ex.User.ID || seen[row.RunID] {
			continue
		}
		seen[row.RunID] = true
		value, _, err := e.compactRun(ctx, ex, resolved, row.RunID, 0, false)
		if err != nil {
			if errors.Is(err, compaction.ErrNoReduction) {
				continue
			}
			if capacityError(err) {
				capacityErr = err
				continue
			}
			return false, err
		}
		if value != nil {
			return true, nil
		}
	}
	return false, capacityErr
}

// shortenHistory 缩小准备窗口，后续页面仍从已成功激活的摘要边界重新读取，不丢弃历史。
func shortenHistory(ex *conversation.Execution) bool {
	var starts []int
	for i, row := range ex.History {
		if row.Role == "user" && row.ID != ex.User.ID {
			starts = append(starts, i)
		}
	}
	if len(starts) <= ex.Run.SettingsSnapshot.Compaction.RetainTurns+1 {
		return false
	}
	boundary := starts[max(len(starts)/2, ex.Run.SettingsSnapshot.Compaction.RetainTurns+1)]
	rows := append([]conversation.Message(nil), ex.History[:boundary]...)
	if ex.User.ID != "" {
		rows = append(rows, ex.User)
	}
	ex.History, ex.HistoryMore = rows, true
	return true
}

func (e *Engine) prepareHistory(ctx context.Context, ex *conversation.Execution, resolved agent.Resolved, manual bool) (map[string][]model.Message, []model.Message, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		floor, ceiling := historyBoundary(*ex)
		traces, err := e.repository.ContextTranscript(ctx, ex.Conversation.ID, floor, ceiling)
		var messages []model.Message
		if err == nil {
			messages, err = history.Assemble(ex.History, traces)
		}
		if err != nil {
			if !capacityError(err) || (!manual && !autoCompaction(*ex)) {
				return nil, nil, err
			}
			changed, compactErr := e.compactHistorical(ctx, *ex, resolved)
			if compactErr != nil && !capacityError(compactErr) {
				return nil, nil, compactErr
			}
			if changed {
				continue
			}
			if shortenHistory(ex) {
				continue
			}
			if compactErr != nil {
				return nil, nil, compactErr
			}
			return nil, nil, err
		}
		if !ex.HistoryMore {
			return traces, messages, nil
		}
		if !manual && !autoCompaction(*ex) {
			return nil, nil, model.ErrInputLimit
		}
		retain := ex.Run.SettingsSnapshot.Compaction.RetainTurns
		if ex.User.ID != "" {
			retain++
		}
		changed, err := e.compactOld(ctx, ex, resolved, traces, retain)
		if err != nil && !capacityError(err) {
			return nil, nil, err
		}
		if !changed {
			changed, compactErr := e.compactHistorical(ctx, *ex, resolved)
			if compactErr != nil {
				return nil, nil, compactErr
			}
			if changed {
				continue
			}
			if err != nil {
				return nil, nil, err
			}
			return nil, nil, compaction.ErrNoReduction
		}
		if err := e.reloadHistory(ctx, ex); err != nil {
			return nil, nil, err
		}
	}
}

func (c *executionContext) messages() []model.Message {
	messages := make([]model.Message, 0, len(c.past)+len(c.current))
	messages = append(messages, c.past...)
	return append(messages, c.current...)
}

// fit 同时用于模型调用前和工具结果追加后；remaining 仅构造容量占位，不执行或持久化。
func (c *executionContext) fit(ctx context.Context, e *Engine, ex *conversation.Execution, resolved agent.Resolved, fixed []model.Message, definitions []model.ToolDefinition, meter *tokens.Meter, before int64, remaining []model.ToolCall) error {
	for {
		preview := append(fixed, contextMessages(ex.Context, c.messages())...)
		for _, call := range remaining {
			preview = append(preview, model.Message{Role: model.RoleTool, ToolCallID: call.ID, Content: "{}"})
			if len(preview) > 256 {
				break
			}
		}
		limitErr := inputCapacity(ctx, *ex, resolved, preview, definitions, meter)
		if limitErr == nil {
			return nil
		}
		if !capacityError(limitErr) || !autoCompaction(*ex) {
			return limitErr
		}
		changed, err := e.compactOld(ctx, ex, resolved, c.traces, ex.Run.SettingsSnapshot.Compaction.RetainTurns+1)
		if err != nil && !capacityError(err) {
			return err
		}
		if changed {
			c.past, err = history.Assemble(ex.History, c.traces)
			if err != nil {
				return err
			}
			continue
		}
		checkpoint, consumed, err := e.compactRun(ctx, *ex, resolved, ex.Run.ID, before, before == 0)
		if err != nil && !capacityError(err) {
			return err
		}
		if checkpoint != nil {
			offset := consumed
			if checkpoint.BaseID != "" {
				offset++
			}
			if offset > len(c.current) {
				return model.ErrInvalidResponse
			}
			c.current = append([]model.Message{checkpoint.Message()}, c.current[offset:]...)
			continue
		}
		changed, historicalErr := e.compactHistorical(ctx, *ex, resolved)
		if historicalErr != nil {
			return historicalErr
		}
		if !changed {
			if err != nil {
				return err
			}
			return limitErr
		}
		c.traces, c.past, err = e.prepareHistory(ctx, ex, resolved, false)
		if err != nil {
			return err
		}
	}
}
