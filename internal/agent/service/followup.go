package service

import (
	"context"
	"slices"
	"sync"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/executor"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/conversation/history"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/policy"
)

type runSteering struct {
	runID      string
	mu         sync.Mutex
	generation uint64
	sealed     bool
	cancel     context.CancelCauseFunc
}

// SteeringLease 固定本次请求瞄准的执行。原地恢复时，同一 run ID 会对应另一个租约。
func (e *Engine) SteeringLease(runID string) *executor.Lease {
	lease := e.manager.ActiveLease(runID)
	if e.steeringState(lease) == nil {
		return nil
	}
	return lease
}

// AcceptSteering 先提交数据、发布用户消息，再唤醒当前模型调用。
func (e *Engine) AcceptSteering(lease *executor.Lease, write func() (conversation.FollowUp, error)) error {
	state := e.steeringState(lease)
	if state == nil {
		return conversation.ErrFollowUpChanged
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.sealed {
		return conversation.ErrFollowUpChanged
	}
	followUp, err := write()
	if err != nil {
		return err
	}
	e.publishSteering("message.created", followUp.Receipt)
	e.FollowUpsUpdated(followUp.ConversationID, state.runID)
	state.generation++
	if state.cancel != nil {
		state.cancel(conversation.ErrSteered)
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

func (e *Engine) registerSteering(lease *executor.Lease, runID string) *runSteering {
	state := &runSteering{runID: runID}
	e.steeringMu.Lock()
	e.steering[lease] = state
	e.steeringMu.Unlock()
	return state
}

func (e *Engine) steeringState(lease *executor.Lease) *runSteering {
	e.steeringMu.Lock()
	defer e.steeringMu.Unlock()
	return e.steering[lease]
}

// removeSteering 在封存后释放登记，已取得旧状态的请求仍会看到 sealed。
func (e *Engine) removeSteering(lease *executor.Lease) {
	e.steeringMu.Lock()
	delete(e.steering, lease)
	e.steeringMu.Unlock()
}

// snapshotSteering 让引导快照与受理互斥，避免在创建模型步骤期间漏掉新引导。
func (e *Engine) snapshotSteering(ctx context.Context, state *runSteering) ([]conversation.FollowUp, uint64, error) {
	state.mu.Lock()
	defer state.mu.Unlock()
	pending, err := e.conversations.PendingSteering(ctx, state.runID)
	return pending, state.generation, err
}

// samplingContext 只取消模型调用，保留当前执行、已启动的工具和用户取消语义。
func (state *runSteering) samplingContext(ctx context.Context, generation uint64) (context.Context, func()) {
	request, cancel := context.WithCancelCause(ctx)
	state.mu.Lock()
	if state.generation != generation {
		cancel(conversation.ErrSteered)
	}
	state.cancel = cancel
	state.mu.Unlock()
	return request, func() {
		state.mu.Lock()
		state.cancel = nil
		state.mu.Unlock()
		cancel(nil)
	}
}

func (e *Engine) sealSteering(ctx context.Context, state *runSteering) []conversation.FollowUp {
	state.mu.Lock()
	state.sealed = true
	state.mu.Unlock()
	pending, err := e.conversations.PendingSteering(ctx, state.runID)
	if err != nil {
		e.logger.Error("读取待恢复引导失败", "module", "run", "run_id", state.runID)
	}
	return pending
}

func (e *Engine) steeringRequeued(ex conversation.Execution, pending []conversation.FollowUp) {
	if len(pending) == 0 {
		return
	}
	ids := make([]string, len(pending))
	for i, followUp := range pending {
		ids[i] = followUp.ID
	}
	_ = e.emit(agent.Event{Type: "conversation.steering_requeued", ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, RemovedMessageIDs: ids})
}

func (e *Engine) FollowUpsUpdated(conversationID, runID string) {
	if err := e.emit(agent.Event{Type: "conversation.follow_ups_updated", ConversationID: conversationID, RunID: runID}); err != nil {
		e.logger.Warn("发布后续消息更新失败", "module", "conversation", "conversation_id", conversationID)
	}
}

// beforeTool 按同一顺序处理引导与工具的持久化启动边界。
func (e *Engine) beforeTool(ctx context.Context, state *runSteering, start func() error) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	pending, err := e.conversations.PendingSteering(ctx, state.runID)
	if err != nil {
		return operationStorageError(ctx, err)
	}
	if len(pending) > 0 {
		return conversation.ErrSteered
	}
	return start()
}

func (e *Engine) sealAnswer(ctx context.Context, state *runSteering) (bool, error) {
	state.mu.Lock()
	defer state.mu.Unlock()
	pending, err := e.conversations.PendingSteering(ctx, state.runID)
	if err != nil {
		return false, operationStorageError(ctx, err)
	}
	if len(pending) != 0 {
		return false, nil
	}
	state.sealed = true
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
