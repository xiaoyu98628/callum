package service

import (
	"context"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/conversation/history"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/policy"
)

// AcceptSteering serializes acceptance against final completion and pre-tool boundaries.
func (e *Engine) AcceptSteering(runID string, write func() error) error {
	e.steeringMu.Lock()
	defer e.steeringMu.Unlock()
	if e.sealedRuns[runID] {
		return conversation.ErrFollowUpChanged
	}
	return write()
}

func (e *Engine) GetRun(ctx context.Context, id string) (agent.Run, error) {
	return e.repository.Get(ctx, id)
}

func (e *Engine) FollowUpsUpdated(conversationID, runID string) {
	if err := e.emit(agent.Event{Type: "conversation.follow_ups_updated", ConversationID: conversationID, RunID: runID}); err != nil {
		e.logger.Warn("发布后续消息更新失败", "module", "conversation", "conversation_id", conversationID)
	}
}

// beforeTool 按同一顺序处理引导与工具的持久化启动边界。
func (e *Engine) beforeTool(ctx context.Context, runID string, start func() error) error {
	e.steeringMu.Lock()
	defer e.steeringMu.Unlock()
	pending, err := e.conversations.PendingSteering(ctx, runID)
	if err != nil {
		return operationStorageError(ctx, err)
	}
	if len(pending) > 0 {
		return conversation.ErrSteered
	}
	return start()
}

func (e *Engine) sealAnswer(ctx context.Context, runID string) (bool, error) {
	e.steeringMu.Lock()
	defer e.steeringMu.Unlock()
	pending, err := e.conversations.PendingSteering(ctx, runID)
	if err != nil {
		return false, operationStorageError(ctx, err)
	}
	if len(pending) != 0 {
		return false, nil
	}
	e.sealedRuns[runID] = true
	return true, nil
}

func steeringMessages(items []conversation.FollowUp) []model.Message {
	out := make([]model.Message, 0, len(items))
	for _, item := range items {
		out = append(out, model.Message{Role: model.RoleUser, Blocks: history.ModelContent(item.Content)})
	}
	return out
}

func (e *Engine) skipSteeredCall(ctx context.Context, ex conversation.Execution, parent agent.Step, call model.ToolCall) (model.Message, error) {
	step, err := e.createToolStep(ctx, ex, parent, call, nil, policy.Decision{Action: policy.Deny, Reason: "steered"}, nil)
	if err != nil {
		return model.Message{}, err
	}
	if err := e.event(ex, "step.created", step.ID, "", step); err != nil {
		return model.Message{}, err
	}
	return e.finishTool(ctx, ex, step, call, tool.Result{Outcome: tool.OutcomeNotApplied}, conversation.ErrSteered)
}

// 引导附在所属模型步骤上，供后续对话按实际使用的位置恢复。
func (e *Engine) applySteering(ctx context.Context, ex conversation.Execution, stepID string, items []conversation.FollowUp) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	cleanup, cancel := cleanupContext(ctx)
	defer cancel()
	if err := e.conversations.ApplySteering(cleanup, ex.Run.ID, stepID, ids); err != nil {
		return operationStorageError(ctx, err)
	}
	e.FollowUpsUpdated(ex.Conversation.ID, ex.Run.ID)
	return nil
}
