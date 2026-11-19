package service

import (
	"context"
	"slices"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/conversation/history"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/policy"
)

type runSteering struct {
	generation uint64
	sealed     bool
	cancel     context.CancelCauseFunc
}

// AcceptSteering 先提交数据、发布用户消息，再唤醒当前模型调用。
func (e *Engine) AcceptSteering(runID string, write func() (conversation.FollowUp, error)) error {
	e.steeringMu.Lock()
	defer e.steeringMu.Unlock()
	state := e.steering[runID]
	if state != nil && state.sealed {
		return conversation.ErrFollowUpChanged
	}
	followUp, err := write()
	if err != nil {
		return err
	}
	e.publishSteering("message.created", followUp.Receipt)
	e.FollowUpsUpdated(followUp.ConversationID, runID)
	if state != nil {
		state.generation++
		if state.cancel != nil {
			state.cancel(conversation.ErrSteered)
		}
	}
	return nil
}

func (e *Engine) publishSteering(kind string, message *conversation.Message) {
	if message != nil {
		if err := e.emit(agent.Event{Type: kind, ConversationID: message.ConversationID, RunID: message.RunID, MessageID: message.ID, Data: *message}); err != nil {
			e.logger.Warn("发布引导消息失败", "module", "conversation", "conversation_id", message.ConversationID)
		}
	}
}

func (e *Engine) beginSteering(runID string) {
	e.steeringMu.Lock()
	e.steering[runID] = &runSteering{}
	e.steeringMu.Unlock()
}

func (e *Engine) endSteering(runID string) {
	e.steeringMu.Lock()
	delete(e.steering, runID)
	e.steeringMu.Unlock()
}

// snapshotSteering 让引导快照与受理互斥，避免在创建模型步骤期间漏掉新引导。
func (e *Engine) snapshotSteering(ctx context.Context, runID string) ([]conversation.FollowUp, uint64, error) {
	e.steeringMu.Lock()
	defer e.steeringMu.Unlock()
	pending, err := e.conversations.PendingSteering(ctx, runID)
	return pending, e.steering[runID].generation, err
}

// samplingContext 只取消模型调用，保留当前执行、已启动的工具和用户取消语义。
func (e *Engine) samplingContext(ctx context.Context, runID string, generation uint64) (context.Context, func()) {
	request, cancel := context.WithCancelCause(ctx)
	e.steeringMu.Lock()
	state := e.steering[runID]
	if state.generation != generation {
		cancel(conversation.ErrSteered)
	}
	state.cancel = cancel
	e.steeringMu.Unlock()
	return request, func() {
		e.steeringMu.Lock()
		state.cancel = nil
		e.steeringMu.Unlock()
		cancel(nil)
	}
}

func (e *Engine) sealSteering(ctx context.Context, runID string) ([]conversation.FollowUp, error) {
	e.steeringMu.Lock()
	defer e.steeringMu.Unlock()
	e.steering[runID].sealed = true
	return e.conversations.PendingSteering(ctx, runID)
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
	e.steering[runID].sealed = true
	return true, nil
}

func steeringMessages(items []conversation.FollowUp) []model.Message {
	out := make([]model.Message, 0, len(items))
	for _, item := range items {
		blocks := slices.Clone(history.ModelContent(item.Content))
		marker := "以下是用户在当前任务执行期间发出的引导。以较新的明确要求为准，调整与它冲突的计划、范围和交付内容。继续完成原任务中仍适用的部分；如果用户要求停止或切换任务，按其意图执行。"
		if len(blocks) > 0 && blocks[0].Type == "text" {
			blocks[0].Text = marker + "\n\n" + blocks[0].Text
		} else {
			blocks = append([]model.Block{{Type: "text", Text: marker}}, blocks...)
		}
		out = append(out, model.Message{Role: model.RoleUser, Blocks: blocks})
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
	messages, err := e.conversations.ApplySteering(cleanup, ex.Run.ID, stepID, ids)
	if err != nil {
		return operationStorageError(ctx, err)
	}
	for i := range messages {
		e.publishSteering("message.updated", &messages[i])
	}
	return nil
}
