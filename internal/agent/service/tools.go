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
	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/tool"
	toolexecutor "github.com/xiaoyu98628/callum/internal/tool/executor"
	"github.com/xiaoyu98628/callum/internal/tool/policy"
)

func (e *Engine) callTool(ctx context.Context, ex conversation.Execution, parent agent.Step, call model.ToolCall, denied map[string]bool) (message model.Message, retErr error) {
	scope, operationErr := e.toolScope(ctx, ex)
	var prepared *toolexecutor.Prepared
	if operationErr == nil {
		prepared, operationErr = e.options.Tools.Prepare(ctx, scope, call.Name, call.Arguments)
	}
	var step agent.Step
	var output json.RawMessage
	// 先关闭固定根句柄，再保存操作结果；关闭失败也保留在受控错误和执行状态中。
	defer func() {
		if prepared != nil {
			closeErr := prepared.Close()
			operationErr = errors.Join(operationErr, closeErr)
			if step.ID == "" {
				retErr = errors.Join(retErr, closeErr)
			}
		}
		if step.ID != "" {
			var finishErr error
			message, finishErr = e.finishTool(ctx, ex, step, call, output, operationErr)
			retErr = errors.Join(retErr, finishErr)
		}
		if err := ctx.Err(); err != nil {
			retErr = errors.Join(retErr, err)
		}
	}()
	decision := toolDecision(prepared, denied, ex.Run.SettingsSnapshot)
	var err error
	step, err = e.createToolStep(ctx, ex, parent, call, prepared, decision)
	if err != nil {
		return message, err
	}
	if err := e.event(ex, "step.created", step.ID, "", step); err != nil {
		operationErr = err
		return message, err
	}
	if operationErr != nil {
		return
	}
	approvalID, err := e.authorizeTool(ctx, ex, &step, prepared, decision, denied)
	if err != nil {
		operationErr = err
		if errors.Is(err, tool.ErrDenied) {
			return
		}
		return message, err
	}
	if err := ctx.Err(); err != nil {
		operationErr = err
		return message, err
	}
	current, err := e.toolScope(ctx, ex)
	if err != nil || current != scope {
		operationErr = tool.ErrDenied
		return
	}
	claimed, err := e.repository.StartTool(ctx, ex.Run.ID, step.ID, approvalID, prepared.Hash())
	if err != nil {
		operationErr = err
		return message, operationStorageError(ctx, err)
	}
	step = claimed
	if err := e.event(ex, "run.updated", "", "", ex.Run); err != nil {
		operationErr = err
		return message, err
	}
	if err := e.event(ex, "step.updated", step.ID, "", step); err != nil {
		operationErr = err
		return message, err
	}
	output, operationErr = e.options.Tools.Execute(ctx, prepared, prepared.Hash())
	return
}

func toolDecision(prepared *toolexecutor.Prepared, denied map[string]bool, snapshot *settings.Snapshot) policy.Decision {
	if snapshot == nil || !snapshot.Valid() {
		return policy.Decision{Action: policy.Deny, Reason: "invalid_settings_snapshot"}
	}
	if prepared == nil {
		return policy.Decision{Action: policy.Deny, Reason: "operation_validation_failed"}
	}
	if denied[prepared.Hash()] {
		return policy.Decision{Action: policy.Deny, Reason: "previously_rejected"}
	}
	return policy.Evaluate(snapshot.ApprovalPolicy, prepared.Definition().Effects)
}

func (e *Engine) createToolStep(ctx context.Context, ex conversation.Execution, parent agent.Step, call model.ToolCall, prepared *toolexecutor.Prepared, decision policy.Decision) (agent.Step, error) {
	input := map[string]any{"arguments": call.Arguments, "authorization": decision}
	if snapshot := ex.Run.SettingsSnapshot; snapshot != nil {
		input["approval_policy"], input["approvals_reviewer"], input["settings_revision"] = snapshot.ApprovalPolicy, snapshot.ApprovalsReviewer, snapshot.Revision
	}
	var definition json.RawMessage
	if prepared != nil {
		input["request"], input["request_hash"] = prepared.Request(), prepared.Hash()
		var err error
		definition, err = json.Marshal(prepared.Definition())
		if err != nil {
			return agent.Step{}, err
		}
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return agent.Step{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return agent.Step{}, err
	}
	now := time.Now().UnixMilli()
	step, err := e.repository.CreateStep(ctx, agent.Step{ID: id.String(), ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, ParentStepID: new(parent.ID), Type: "tool_call", Name: call.Name, Definition: definition, Status: agent.Pending, Input: inputJSON, ToolCallID: new(call.ID), CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return step, operationStorageError(ctx, err)
	}
	return step, nil
}

func (e *Engine) authorizeTool(ctx context.Context, ex conversation.Execution, step *agent.Step, prepared *toolexecutor.Prepared, decision policy.Decision, denied map[string]bool) (string, error) {
	switch decision.Action {
	case policy.Allow:
		return "", nil
	case policy.Ask:
		// 审批策略只决定是否询问；审批人独立校验，不能回退成自动放行。
		if ex.Run.SettingsSnapshot == nil || ex.Run.SettingsSnapshot.ApprovalsReviewer != policy.User {
			return "", tool.ErrDenied
		}
	default:
		return "", tool.ErrDenied
	}
	a, err := e.options.Approvals.Request(ctx, ex.Run.ID, step.ID, prepared.Request(), prepared.Hash())
	if err != nil {
		return "", operationStorageError(ctx, err)
	}
	step.Status, step.UpdatedAt = agent.AwaitingApproval, time.Now().UnixMilli()
	if err := e.event(ex, "step.updated", step.ID, "", *step); err != nil {
		return "", err
	}
	paused := ex.Run
	paused.Status, paused.UpdatedAt = agent.AwaitingApproval, step.UpdatedAt
	if err := e.event(ex, "run.updated", "", "", paused); err != nil {
		return "", err
	}
	decided, err := e.options.Approvals.Wait(ctx, a.ID)
	if err != nil {
		return "", err
	}
	if decided.Status == agent.ApprovalApproved {
		return decided.ID, nil
	}
	denied[prepared.Hash()] = true
	if err := e.repository.Resume(ctx, ex.Run.ID); err != nil {
		return "", operationStorageError(ctx, err)
	}
	if err := e.event(ex, "run.updated", "", "", ex.Run); err != nil {
		return "", err
	}
	return "", tool.ErrDenied
}

func (e *Engine) finishTool(ctx context.Context, ex conversation.Execution, step agent.Step, call model.ToolCall, output json.RawMessage, operationErr error) (model.Message, error) {
	if operationErr != nil {
		output = toolFailure(operationErr)
	}
	if output == nil {
		output = json.RawMessage(`{}`)
	}
	message := model.Message{Role: model.RoleTool, Content: string(output), ToolCallID: call.ID}
	envelope, err := json.Marshal(map[string]json.RawMessage{"result": output})
	if err != nil {
		return message, err
	}
	envelope, err = withContextMessage(envelope, message)
	if err != nil {
		return message, err
	}
	now := time.Now().UnixMilli()
	step.Status, step.Output, step.UpdatedAt, step.FinishedAt = agent.Completed, envelope, now, &now
	if operationErr != nil {
		step.Status = agent.Failed
		if errors.Is(operationErr, context.Canceled) {
			step.Status = agent.Cancelled
		}
		var value struct {
			Error struct{ Kind, Message string } `json:"error"`
		}
		if err := json.Unmarshal(output, &value); err != nil {
			return message, err
		}
		step.ErrorKind, step.ErrorMessage = &value.Error.Kind, &value.Error.Message
	}
	cleanup, cancel := cleanupContext(ctx)
	defer cancel()
	if err := e.repository.SaveStep(cleanup, step); err != nil {
		return message, errors.Join(agent.ErrStorage, err)
	}
	return message, e.event(ex, "step.updated", step.ID, "", step)
}

func (e *Engine) toolScope(ctx context.Context, ex conversation.Execution) (tool.Scope, error) {
	if ex.Conversation.WorkspaceID == nil {
		return tool.Scope{}, tool.ErrDenied
	}
	workspace, err := e.options.Workspace(ctx, *ex.Conversation.WorkspaceID)
	if err != nil {
		return tool.Scope{}, tool.ErrUnavailable
	}
	if workspace.DeletedAt != nil || workspace.ArchivedAt != nil {
		return tool.Scope{}, tool.ErrDenied
	}
	return tool.Scope{ID: workspace.ID, RootPath: workspace.RootPath}, nil
}
