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

// summaryModel 使用统一调用槽位和超时，原始结果、usage 只保存到具体模型步骤。
func (e *Engine) summaryModel(ctx context.Context, ex conversation.Execution, resolved agent.Resolved, parent agent.Step, messages []model.Message, versionID string) (result model.Response, retErr error) {
	id, err := uuid.NewV7()
	if err != nil {
		return result, err
	}
	metadata := map[string]any{"purpose": "context_summary", "generated_context_version_id": versionID, "protocol": resolved.Protocol}
	if ex.Context != nil {
		metadata["context_version_id"] = ex.Context.ID
	}
	input, err := json.Marshal(metadata)
	if err != nil {
		return result, err
	}
	now := time.Now().UnixMilli()
	step := agent.Step{ID: id.String(), ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, ParentStepID: &parent.ID,
		Type: "model_call", Name: "生成历史摘要", Status: agent.Running, Input: input, Model: &resolved.Snapshot, CreatedAt: now, StartedAt: &now, UpdatedAt: now}
	if err := modelMetadata(&step, messages, nil, resolved, false); err != nil {
		return result, err
	}
	if err := tokenMetadata(&step, ex, resolved, messages, nil); err != nil {
		return result, err
	}
	step, err = e.repository.CreateStep(ctx, step)
	if err != nil {
		return result, operationStorageError(ctx, err)
	}
	if err := e.event(ex, "step.created", step.ID, "", step); err != nil {
		return result, err
	}
	callCtx, cancel := context.WithTimeout(ctx, e.options.CallTimeout)
	defer cancel()
	retErr = e.manager.Call(callCtx, func() error {
		var err error
		result, err = resolved.Client.Generate(callCtx, model.Request{Model: resolved.Snapshot.RemoteModelID, CaptureResult: true, AllowImageInput: resolved.ImageInput, Messages: messages})
		return err
	})
	if err := callCtx.Err(); err != nil {
		retErr = errors.Join(retErr, err)
	}
	fillFailure(&result, resolved.Protocol, retErr)
	step.Status = agent.Completed
	if retErr != nil {
		status, kind, message := failure(retErr)
		step.Status, step.ErrorKind, step.ErrorMessage = status, &kind, &message
	}
	now = time.Now().UnixMilli()
	step.Output, step.Usage, step.UpdatedAt, step.FinishedAt = result.Output, result.Usage, now, &now
	if result.RequestID != "" {
		step.ProviderRequestID = &result.RequestID
	}
	cleanup, finish := cleanupContext(ctx)
	defer finish()
	if err := e.repository.SaveStep(cleanup, step); err != nil {
		return result, errors.Join(retErr, agent.ErrStorage, err)
	}
	if err := e.event(ex, "step.updated", step.ID, "", step); err != nil {
		return result, errors.Join(retErr, err)
	}
	return result, retErr
}
