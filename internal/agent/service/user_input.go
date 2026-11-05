package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/model"
)

// GetInput 与 Inputs 返回固定问题和已保存回复；不暴露写入步骤的通用能力。
func (e *Engine) GetInput(ctx context.Context, id string) (agent.UserInput, error) {
	if !validID(id) {
		return agent.UserInput{}, agent.ErrInvalid
	}
	step, err := e.repository.GetStep(ctx, id)
	if err != nil {
		return agent.UserInput{}, err
	}
	return agent.InputFromStep(step)
}
func (e *Engine) Inputs(ctx context.Context, runID string, pending bool, limit, offset int) ([]agent.UserInput, error) {
	if !validID(runID) || limit < 1 || limit > 200 || offset < 0 {
		return nil, agent.ErrInvalid
	}
	rows, err := e.repository.UserInputSteps(ctx, runID, pending, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]agent.UserInput, 0, len(rows))
	for _, row := range rows {
		value, err := agent.InputFromStep(row)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

func (e *Engine) SubmitInput(ctx context.Context, id, runID string, answer agent.InputAnswer) (agent.UserInput, error) {
	if !validID(id) || !validID(runID) {
		return agent.UserInput{}, agent.ErrInvalid
	}
	// 落库、发布和唤醒有固定次序，执行器不会先生成下一轮再发布旧的等待状态。
	e.inputMu.Lock()
	defer e.inputMu.Unlock()
	step, changed, err := e.repository.AnswerUserInput(ctx, id, runID, answer)
	if err != nil {
		return agent.UserInput{}, err
	}
	value, err := agent.InputFromStep(step)
	if err != nil {
		return value, err
	}
	if changed {
		publishErr := e.publishInput(ctx, "user_input.updated", step)
		if ch := e.inputSignals[id]; ch != nil {
			select {
			case ch <- struct{}{}:
			default:
			}
		}
		return value, publishErr
	}
	return value, nil
}

func (e *Engine) publishInput(ctx context.Context, name string, step agent.Step) error {
	value, err := agent.InputFromStep(step)
	if err != nil {
		return err
	}
	if err := e.emit(agent.Event{Type: name, ConversationID: step.ConversationID, RunID: step.RunID, StepID: step.ID, Data: value}); err != nil {
		return err
	}
	if err := e.emit(agent.Event{Type: "step.updated", ConversationID: step.ConversationID, RunID: step.RunID, StepID: step.ID, Data: step}); err != nil {
		return err
	}
	run, err := e.repository.Get(ctx, step.RunID)
	if err != nil {
		return err
	}
	if err := e.emit(agent.Event{Type: "run.updated", ConversationID: step.ConversationID, RunID: step.RunID, Data: run}); err != nil {
		return err
	}
	rows, err := e.conversations.Messages(ctx, step.ConversationID, 0, 1)
	if err != nil {
		return err
	}
	if len(rows) != 1 || rows[0].RunID != step.RunID || rows[0].Role != "assistant" {
		return agent.ErrConflict
	}
	return e.emit(agent.Event{Type: "message.updated", ConversationID: step.ConversationID, RunID: step.RunID, MessageID: rows[0].ID, Data: rows[0]})
}

func (e *Engine) askUser(ctx context.Context, ex conversation.Execution, parent agent.Step, call model.ToolCall) (model.Message, error) {
	question, err := agent.ParseInputQuestion(call.Arguments)
	if err != nil {
		return e.invalidQuestion(ctx, ex, parent, call)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return model.Message{}, err
	}
	raw, err := json.Marshal(question)
	if err != nil {
		return model.Message{}, err
	}
	definition, err := json.Marshal(agent.UserInputDefinition())
	if err != nil {
		return model.Message{}, err
	}
	now := time.Now().UnixMilli()
	step, err := e.repository.CreateUserInput(ctx, agent.Step{ID: id.String(), ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, ParentStepID: new(parent.ID), ToolCallID: new(call.ID), Type: "user_input", Name: "等待用户回复", Definition: definition, Input: raw, Status: agent.AwaitingUserInput, CreatedAt: now, UpdatedAt: now, StartedAt: &now})
	if err != nil {
		return model.Message{}, operationStorageError(ctx, err)
	}
	if err := e.event(ex, "step.created", step.ID, "", step); err != nil {
		return model.Message{}, err
	}
	if err := e.publishInput(ctx, "user_input.requested", step); err != nil {
		return model.Message{}, err
	}
	step, err = e.waitInput(ctx, step.ID)
	if err != nil {
		return model.Message{}, err
	}
	var output struct {
		Message model.Message `json:"context_message"`
	}
	if json.Unmarshal(step.Output, &output) != nil || output.Message.Role != model.RoleTool || output.Message.ToolCallID != call.ID {
		return model.Message{}, agent.ErrInvalid
	}
	return output.Message, nil
}

func (e *Engine) waitInput(ctx context.Context, id string) (agent.Step, error) {
	ch := make(chan struct{}, 1)
	e.inputMu.Lock()
	if e.inputSignals[id] != nil {
		e.inputMu.Unlock()
		return agent.Step{}, agent.ErrConflict
	}
	e.inputSignals[id] = ch
	e.inputMu.Unlock()
	defer func() { e.inputMu.Lock(); delete(e.inputSignals, id); e.inputMu.Unlock() }()
	for {
		if err := ctx.Err(); err != nil {
			return e.cancelInput(ctx, id, err)
		}
		step, err := e.repository.GetStep(ctx, id)
		if err != nil {
			return step, err
		}
		if step.Status == agent.Completed {
			return step, nil
		}
		if step.Status != agent.AwaitingUserInput {
			return step, agent.ErrConflict
		}
		select {
		case <-ctx.Done():
			return e.cancelInput(ctx, id, ctx.Err())
		case <-ch:
		}
	}
}
func (e *Engine) cancelInput(ctx context.Context, id string, cause error) (agent.Step, error) {
	cleanup, cancel := cleanupContext(ctx)
	defer cancel()
	step, err := e.repository.CancelUserInput(cleanup, id)
	if err == nil {
		err = e.publishInput(cleanup, "user_input.updated", step)
	}
	return step, errors.Join(cause, err)
}

// 无效提问也保存调用结果，避免后续上下文出现没有对应结果的工具调用。
func (e *Engine) invalidQuestion(ctx context.Context, ex conversation.Execution, parent agent.Step, call model.ToolCall) (model.Message, error) {
	message := model.Message{Role: model.RoleTool, ToolCallID: call.ID, Content: `{"error":{"kind":"invalid_arguments","message":"问题参数无效，需要 question 和 options，最多 8 个不重复选项"}}`}
	output, err := json.Marshal(map[string]any{"context_message": message})
	if err != nil {
		return message, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return message, err
	}
	now := time.Now().UnixMilli()
	input := call.Arguments
	if !json.Valid(input) {
		input = json.RawMessage(`{}`)
	}
	step, err := e.repository.CreateStep(ctx, agent.Step{ID: id.String(), ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, ParentStepID: new(parent.ID), ToolCallID: new(call.ID), Type: "tool_call", Name: agent.UserInputTool, Input: input, Output: output, Status: agent.Failed, CreatedAt: now, UpdatedAt: now, FinishedAt: &now})
	if err != nil {
		return message, operationStorageError(ctx, err)
	}
	return message, e.event(ex, "step.created", step.ID, "", step)
}
