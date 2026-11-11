package service

import (
	"strings"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/tokens"
	"github.com/xiaoyu98628/callum/internal/settings"
)

type liveContextUsage struct {
	value       conversation.ContextUsage
	actual      *int64
	imageTokens int64
	lastEmit    time.Time
}

func copyInt(value *int64) *int64 {
	if value == nil {
		return nil
	}
	return new(*value)
}

func cloneUsage(value conversation.ContextUsage) conversation.ContextUsage {
	value.EstimatedTokens = copyInt(value.EstimatedTokens)
	value.InputLimit = copyInt(value.InputLimit)
	value.ContextWindow = copyInt(value.ContextWindow)
	value.ActualInputTokens = copyInt(value.ActualInputTokens)
	return value
}

func usageBase(id, runID string, resolved agent.Resolved, cfg settings.Compaction) conversation.ContextUsage {
	value := conversation.ContextUsage{ConversationID: id, ModelID: resolved.Snapshot.ID, RunID: runID, State: "preparing", OutputReserve: cfg.OutputReserve, SafetyMargin: cfg.SafetyMargin, EstimationMethod: tokens.Method}
	if resolved.ContextWindow > 0 {
		value.ContextWindow = new(resolved.ContextWindow)
	}
	if resolved.MaxOutputTokens > 0 {
		value.OutputReserve = min(value.OutputReserve, resolved.MaxOutputTokens)
	}
	if budget, err := cfg.InputBudget(resolved.ContextWindow, resolved.MaxOutputTokens); err == nil {
		value.InputLimit = new(budget)
	} else {
		value.Reason = "capacity_unconfigured"
	}
	return value
}

// ContextRevision 用于丢弃晚于运行状态变化才返回的只读预览，不写入业务数据库。
func (e *Engine) ContextRevision() int64 { return e.usageRevision.Load() }

func (e *Engine) beginContextUsage(ex conversation.Execution, resolved agent.Resolved) {
	if ex.Run.SettingsSnapshot == nil || ex.Run.SettingsSnapshot.Compaction == nil {
		return
	}
	value := usageBase(ex.Conversation.ID, ex.Run.ID, resolved, ex.Run.SettingsSnapshot.Compaction.Compaction)
	e.usageMu.Lock()
	e.usage[ex.Conversation.ID] = liveContextUsage{value: value, imageTokens: ex.Run.SettingsSnapshot.Compaction.ImageTokenEstimate}
	e.usageMu.Unlock()
	e.storeContextUsage(value, true)
}

func (e *Engine) emitContextUsage(value conversation.ContextUsage) {
	if err := e.emit(agent.Event{Type: "conversation.context_usage.updated", ConversationID: value.ConversationID, RunID: value.RunID, Data: value}); err != nil {
		e.logger.Warn("发布上下文占用失败", "module", "context_usage", "conversation_id", value.ConversationID, "run_id", value.RunID)
	}
}

// storeContextUsage 只保存有界活动执行的数字快照；高频生成通知最多每 100ms 发布一次。
// 每次分片仍更新查询快照，既不保留正文副本，也不创建后台定时任务。
func (e *Engine) storeContextUsage(value conversation.ContextUsage, force bool) {
	now := time.Now()
	e.usageMu.Lock()
	live, ok := e.usage[value.ConversationID]
	if !ok || live.value.RunID != value.RunID {
		e.usageMu.Unlock()
		return
	}
	value.Revision, value.UpdatedAt = e.usageRevision.Add(1), now.UnixMilli()
	value.ActualInputTokens = copyInt(live.actual)
	live.value = cloneUsage(value)
	publish := force || now.Sub(live.lastEmit) >= 100*time.Millisecond
	if publish {
		live.lastEmit = now
	}
	e.usage[value.ConversationID] = live
	e.usageMu.Unlock()
	if publish {
		e.emitContextUsage(value)
	}
}

func (e *Engine) endContextUsage(ex conversation.Execution) {
	e.usageMu.Lock()
	live, ok := e.usage[ex.Conversation.ID]
	if ok && live.value.RunID == ex.Run.ID {
		delete(e.usage, ex.Conversation.ID)
	} else {
		ok = false
	}
	e.usageMu.Unlock()
	if !ok {
		return
	}
	value := cloneUsage(live.value)
	value.State, value.Reason, value.EstimatedTokens = "refresh_required", "", nil
	value.Revision, value.UpdatedAt = e.usageRevision.Add(1), time.Now().UnixMilli()
	e.emitContextUsage(value)
}

func withDraft(value conversation.ContextUsage, draft string, imageCount int, imageTokens int64) conversation.ContextUsage {
	value = cloneUsage(value)
	value.DraftTokens = int64(imageCount) * imageTokens
	if draft = strings.TrimSpace(draft); draft != "" {
		value.DraftTokens += tokens.Estimate([]model.Message{{Role: model.RoleUser, Content: draft}}, nil, 0) - 16
	}
	if value.EstimatedTokens != nil {
		*value.EstimatedTokens += value.DraftTokens
	}
	return value
}

// LiveContextUsage 只读取所属执行的数字快照，返回值与运行缓存相互隔离。
func (e *Engine) LiveContextUsage(in conversation.ContextUsageRequest) (conversation.ContextUsage, bool, error) {
	e.usageMu.RLock()
	live, ok := e.usage[in.ConversationID]
	value := cloneUsage(live.value)
	e.usageMu.RUnlock()
	if !ok {
		return value, false, nil
	}
	if in.ModelID != "" && in.ModelID != value.ModelID {
		return value, true, conversation.ErrBusy
	}
	return withDraft(value, in.Draft, in.DraftImages, live.imageTokens), true, nil
}

func (e *Engine) recordContext(ex conversation.Execution, resolved agent.Resolved, messages []model.Message, definitions []model.ToolDefinition, meter *tokens.Meter, force bool) {
	if ex.Run.SettingsSnapshot == nil || ex.Run.SettingsSnapshot.Compaction == nil {
		return
	}
	cfg := ex.Run.SettingsSnapshot.Compaction
	value := usageBase(ex.Conversation.ID, ex.Run.ID, resolved, cfg.Compaction)
	var estimate tokens.EstimateResult
	if meter != nil {
		estimate = meter.Estimate(messages, definitions, cfg.ImageTokenEstimate)
	} else {
		estimate = tokens.EstimateResult{Tokens: tokens.Estimate(messages, definitions, cfg.ImageTokenEstimate), Method: tokens.Method}
	}
	value.State, value.EstimatedTokens, value.EstimationMethod = "ready", new(estimate.Tokens), estimate.Method
	e.storeContextUsage(value, force)
}

type streamContextUsage struct {
	engine          *Engine
	base            conversation.ContextUsage
	text, reasoning tokens.TextCounter
}

func (e *Engine) contextStream(ex conversation.Execution) streamContextUsage {
	value, _, _ := e.LiveContextUsage(conversation.ContextUsageRequest{ConversationID: ex.Conversation.ID})
	return streamContextUsage{engine: e, base: value}
}

func (s *streamContextUsage) add(text string, reasoning bool) {
	if reasoning {
		s.reasoning.Add(text)
	} else {
		s.text.Add(text)
	}
	if s.base.EstimatedTokens == nil {
		return
	}
	value := cloneUsage(s.base)
	*value.EstimatedTokens += 8 + tokens.Text(string(model.RoleAssistant)) + s.text.Tokens() + s.reasoning.Tokens()
	s.engine.storeContextUsage(value, false)
}

func (e *Engine) observeInputTokens(ex conversation.Execution, actual *int64) {
	e.usageMu.Lock()
	if live, ok := e.usage[ex.Conversation.ID]; ok && live.value.RunID == ex.Run.ID {
		live.actual = nil
		if actual != nil && *actual >= 0 && *actual <= 1_000_000_000_000 {
			live.actual = copyInt(actual)
		}
		live.value.ActualInputTokens = copyInt(live.actual)
		e.usage[ex.Conversation.ID] = live
	}
	e.usageMu.Unlock()
}
