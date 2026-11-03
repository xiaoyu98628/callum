package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/conversation/history"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/run"
)

// TitleTimeout 限制辅助标题请求，主回答结束时还会提前取消它。
const TitleTimeout = 10 * time.Second

func (e *Engine) title(parent context.Context, ex conversation.Execution, resolved run.Resolved) {
	ctx, cancel := context.WithTimeout(parent, TitleTimeout)
	defer cancel()
	err := e.titleStep(ctx, ex, resolved)
	if err != nil && ctx.Err() == nil {
		e.logger.Warn("辅助标题任务未完成", "module", "run", "run_id", ex.Run.ID)
	}
}

func (e *Engine) titleStep(ctx context.Context, ex conversation.Execution, resolved run.Resolved) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	input, err := json.Marshal(map[string]any{"purpose": "conversation_title", "protocol": resolved.Protocol, "message_id": ex.User.ID, "fingerprint": history.Fingerprint(ex.User.Content)})
	if err != nil {
		return err
	}
	step := run.Step{ID: id.String(), ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, Type: "model_call", Name: "生成会话标题", Status: run.Running, Input: input, Model: &resolved.Snapshot, CreatedAt: now, StartedAt: &now, UpdatedAt: now}
	step, err = e.repository.CreateStep(ctx, step)
	if err != nil {
		return err
	}
	if err := e.event(ex, "step.created", step.ID, "", step); err != nil {
		return err
	}
	result, callErr := resolved.Client.Generate(ctx, model.Request{Model: resolved.Snapshot.RemoteModelID, CaptureResult: true, AllowImageInput: resolved.ImageInput, Messages: []model.Message{{Role: model.RoleSystem, Content: "请为用户的问题生成一个简短会话标题，不超过30个Unicode字符，只返回一行标题，不要回答问题、不要附加解释。"}, {Role: model.RoleUser, Blocks: ex.User.Content}}})
	if ctx.Err() != nil {
		callErr = errors.Join(ctx.Err(), callErr)
	}
	fillFailure(&result, resolved.Protocol, callErr)
	status := run.Completed
	if callErr != nil {
		var kind, message string
		status, kind, message = failure(callErr)
		step.ErrorKind, step.ErrorMessage = &kind, &message
	}
	now = time.Now().UnixMilli()
	step.Status, step.Output, step.Usage, step.UpdatedAt, step.FinishedAt = status, result.Output, result.Usage, now, &now
	if result.RequestID != "" {
		step.ProviderRequestID = &result.RequestID
	}
	cleanup, finishCancel := cleanupContext(ctx)
	defer finishCancel()
	if err := e.repository.SaveStep(cleanup, step); err != nil {
		return err
	}
	_ = e.event(ex, "step.updated", step.ID, "", step)
	if callErr != nil {
		return nil
	}
	title := strings.TrimSpace(result.Text)
	if index := strings.IndexAny(title, "\r\n"); index >= 0 {
		title = strings.TrimSpace(title[:index])
	}
	title = strings.Trim(title, "\"“”")
	if title == "" || len(result.Text) > 4096 {
		return nil
	}
	title = history.Title([]model.Block{{Type: "text", Text: title}})
	conv, applied, err := e.conversations.ApplyTitle(cleanup, ex.Conversation.ID, ex.User.ID, history.Fingerprint(ex.User.Content), title)
	if err != nil {
		return err
	}
	if applied {
		return e.event(ex, "conversation.updated", "", "", conv)
	}
	return nil
}
