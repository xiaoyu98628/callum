package service

import (
	"context"
	"encoding/base64"
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

func (e *Engine) callTool(ctx context.Context, ex conversation.Execution, resolved agent.Resolved, parent agent.Step, call model.ToolCall, denied, uncertain map[string]bool) (message model.Message, retErr error) {
	scope, operationErr := e.toolScope(ctx, ex)
	var prepared *toolexecutor.Prepared
	if operationErr == nil {
		permissions := tool.ExecutionPermissions{ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, AllowImageInput: resolved.ImageInput}
		if e.options.Attachments != nil {
			if files, ok := e.options.Attachments.Files(ex.Conversation.ID, ex.Run.ID); ok {
				permissions.InputDirectory, permissions.WorkDirectory = files.Inputs, files.Work
			}
		}
		if snapshot := ex.Run.SettingsSnapshot; snapshot != nil {
			permissions.SandboxMode = snapshot.SandboxMode
		}
		prepared, operationErr = e.options.Tools.PrepareWithPermissions(ctx, scope, call.Name, call.Arguments, permissions)
	}
	var step agent.Step
	result := tool.Result{Outcome: tool.OutcomeNotApplied}
	// 先关闭固定调用资源，再保存操作结果；关闭失败也保留在受控错误和执行状态中。
	defer func() {
		if prepared != nil {
			closeErr := prepared.Close()
			operationErr = errors.Join(operationErr, closeErr)
			if result.Outcome == tool.OutcomeUnknown || result.Outcome == tool.OutcomePartial || (result.Outcome == tool.OutcomeApplied && operationErr != nil) {
				uncertain[prepared.Hash()] = true
			} else if operationErr == nil && (result.Outcome == tool.OutcomeApplied || result.Outcome == tool.OutcomeNone) {
				delete(uncertain, prepared.Hash())
			}
			if step.ID == "" {
				retErr = errors.Join(retErr, closeErr)
			}
		}
		if step.ID != "" {
			var finishErr error
			message, finishErr = e.finishTool(ctx, ex, step, call, result, operationErr)
			retErr = errors.Join(retErr, finishErr)
		}
		if err := ctx.Err(); err != nil {
			retErr = errors.Join(retErr, err)
		}
	}()
	decision := toolDecision(prepared, denied, uncertain, ex.Run.SettingsSnapshot)
	var err error
	var directoryIDs []string
	if decision.Action == policy.Ask && decision.Reason == "outside_workspace" && e.options.Approvals != nil {
		directoryIDs, err = e.options.Approvals.MatchDirectories(ctx, ex.Conversation.ID, prepared.Targets(), prepared.Definition().Effects)
		if err != nil {
			return message, operationStorageError(ctx, err)
		}
		decision = policy.ApplyDirectoryGrant(decision, len(directoryIDs) > 0)
	}
	step, err = e.createToolStep(ctx, ex, parent, call, prepared, decision, directoryIDs)
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
	approvalID, err := e.authorizeTool(ctx, ex, resolved, &step, prepared, decision, denied)
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
		if errors.Is(err, tool.ErrDenied) {
			return
		}
		return message, operationStorageError(ctx, err)
	}
	step = claimed
	// 先保存可能已发生操作的边界，保存失败则不调用工具。终态写入失败或崩溃时
	// 依然有保守的未知记录；正常返回再用工具报告的事实替换它。
	checkpoint := tool.OutcomeRecord{Version: tool.OperationVersion, Outcome: tool.UnconfirmedOutcome(prepared.Definition().Effects), RequestHash: prepared.Hash()}
	step.Output, err = json.Marshal(map[string]any{"operation": checkpoint})
	if err == nil {
		err = e.repository.SaveStep(ctx, step)
	}
	if err != nil {
		operationErr = err
		return message, operationStorageError(ctx, err)
	}
	if err := e.event(ex, "run.updated", "", "", ex.Run); err != nil {
		operationErr = err
		return message, err
	}
	if err := e.event(ex, "step.updated", step.ID, "", step); err != nil {
		operationErr = err
		return message, err
	}
	result, operationErr = e.options.Tools.ExecuteStream(ctx, prepared, prepared.Hash(), func(chunk tool.OutputChunk) error {
		// 转为独立字符串后再发布，订阅队列不借用工具的读取缓冲区。
		return e.event(ex, "tool.output", step.ID, "", agent.ToolOutput{Sequence: chunk.Sequence, Stream: chunk.Stream, Encoding: "base64", Data: base64.StdEncoding.EncodeToString(chunk.Data)})
	})
	return
}

func toolDecision(prepared *toolexecutor.Prepared, denied, uncertain map[string]bool, snapshot *settings.Snapshot) policy.Decision {
	if snapshot == nil || !snapshot.Valid() {
		return policy.Decision{Action: policy.Deny, Reason: "invalid_settings_snapshot"}
	}
	if prepared == nil {
		return policy.Decision{Action: policy.Deny, Reason: "operation_validation_failed"}
	}
	return policy.EvaluateOperation(snapshot.ApprovalPolicy, policy.Operation{
		Effects: prepared.Definition().Effects, OutsideWorkspace: prepared.OutsideWorkspace(),
		SandboxMode: snapshot.SandboxMode, Command: prepared.CommandBoundary(),
		PreviouslyRejected: denied[prepared.Hash()], ResultUnconfirmed: uncertain[prepared.Hash()],
	})
}

func (e *Engine) createToolStep(ctx context.Context, ex conversation.Execution, parent agent.Step, call model.ToolCall, prepared *toolexecutor.Prepared, decision policy.Decision, directoryIDs []string) (agent.Step, error) {
	input := map[string]any{"arguments": call.Arguments, "authorization": decision, "operation_version": tool.OperationVersion}
	if len(directoryIDs) > 0 {
		input["directory_grant_ids"] = directoryIDs
	}
	if snapshot := ex.Run.SettingsSnapshot; snapshot != nil {
		input["approval_policy"], input["approvals_reviewer"], input["settings_revision"] = snapshot.ApprovalPolicy, snapshot.ApprovalsReviewer, snapshot.Revision
		input["sandbox_mode"] = snapshot.SandboxMode
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

func (e *Engine) authorizeTool(ctx context.Context, ex conversation.Execution, resolved agent.Resolved, step *agent.Step, prepared *toolexecutor.Prepared, decision policy.Decision, denied map[string]bool) (string, error) {
	switch decision.Action {
	case policy.Allow:
		return "", nil
	case policy.Ask:
		// 审批策略只决定是否询问；审批人独立校验，不能回退成自动放行。
		if ex.Run.SettingsSnapshot == nil || !ex.Run.SettingsSnapshot.Valid() {
			return "", tool.ErrDenied
		}
	default:
		return "", tool.ErrDenied
	}
	var review *agent.ApprovalReview
	if ex.Run.SettingsSnapshot.ApprovalsReviewer == policy.AutoReview && canReviewTool(prepared, decision) {
		var err error
		review, err = e.reviewTool(ctx, ex, resolved, step, prepared)
		if err != nil {
			return "", err
		}
	}
	a, err := e.options.Approvals.Request(ctx, ex.Run.ID, step.ID, prepared.Request(), prepared.Hash(), review)
	if err != nil {
		return "", operationStorageError(ctx, err)
	}
	step.Status, step.UpdatedAt = agent.AwaitingApproval, time.Now().UnixMilli()
	decided := a
	if a.Status == agent.ApprovalPending {
		if err := e.event(ex, "step.updated", step.ID, "", *step); err != nil {
			return "", err
		}
		paused := ex.Run
		paused.Status, paused.UpdatedAt = agent.AwaitingApproval, step.UpdatedAt
		if err := e.event(ex, "run.updated", "", "", paused); err != nil {
			return "", err
		}
		decided, err = e.options.Approvals.Wait(ctx, a.ID)
		if err != nil {
			return "", err
		}
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

func (e *Engine) finishTool(ctx context.Context, ex conversation.Execution, step agent.Step, call model.ToolCall, result tool.Result, operationErr error) (model.Message, error) {
	var input struct {
		RequestHash string `json:"request_hash"`
	}
	if err := json.Unmarshal(step.Input, &input); err != nil {
		return model.Message{}, errors.Join(agent.ErrStorage, err)
	}
	facts := tool.OutcomeRecord{Version: tool.OperationVersion, Outcome: result.Outcome, RequestHash: input.RequestHash}
	fields := map[string]any{"operation": facts, "result": result.Output}
	var failure struct {
		Error struct {
			Kind    string `json:"kind"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if operationErr != nil {
		controlled := toolFailure(operationErr)
		if err := json.Unmarshal(controlled, &failure); err != nil {
			return model.Message{}, err
		}
		fields["error"] = failure.Error
		if result.Output == nil {
			// 保留原有失败时 result.error 的结构；实际输出存在时与错误分别保存。
			fields["result"] = controlled
		}
	}
	envelope, err := json.Marshal(fields)
	if err != nil {
		return model.Message{}, err
	}
	message := model.Message{Role: model.RoleTool, Content: string(envelope), ToolCallID: call.ID}
	if operationErr == nil && len(result.Images) > 0 {
		message.Blocks = append([]model.Block{{Type: "text", Text: message.Content}}, result.Images...)
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
		step.ErrorKind, step.ErrorMessage = &failure.Error.Kind, &failure.Error.Message
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
		return tool.Scope{}, nil
	}
	workspace, err := e.options.Workspace(ctx, *ex.Conversation.WorkspaceID)
	if err != nil {
		return tool.Scope{}, tool.ErrUnavailable
	}
	if workspace.DeletedAt != nil || workspace.ArchivedAt != nil {
		return tool.Scope{}, tool.ErrDenied
	}
	if workspace.RootPath == "" {
		return tool.Scope{}, nil
	}
	return tool.Scope{ID: workspace.ID, RootPath: workspace.RootPath}, nil
}
