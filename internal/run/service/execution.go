package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/run"
	"github.com/xiaoyu98628/callum/internal/run/executor"
)

// Execute 只在受理响应已排队后运行；标题任务在终态前取消并等待退出。
func (e *Engine) Execute(ctx context.Context, lease *executor.Lease, ex conversation.Execution, resolved run.Resolved) {
	err := e.initialEvents(ex)
	var titleWG sync.WaitGroup
	titleCtx, titleCancel := context.WithCancel(ctx)
	if err == nil {
		err = e.manager.Call(ctx, func() error {
			if ex.User.Sequence == 1 && ex.Conversation.TitleSource == "default" {
				// 辅助调用无法立即取得容量或已取消时保留默认标题，不阻塞主回答。
				if release, ok, _ := e.manager.TryAcquireCall(titleCtx); ok {
					titleWG.Go(func() { defer release(); e.title(titleCtx, ex, resolved) })
				}
			}
			_, callErr := e.answer(ctx, lease, ex, resolved)
			return callErr
		})
	}
	titleCancel()
	titleWG.Wait()
	if cancelErr := lease.Finishing(); cancelErr != nil {
		err = errors.Join(cancelErr, err)
	}
	status := run.Completed
	var kind, message *string
	if err != nil {
		var k, m string
		status, k, m = failure(err)
		kind, message = &k, &m
	}
	now := time.Now().UnixMilli()
	ex.Run.Status, ex.Run.ErrorKind, ex.Run.ErrorMessage, ex.Run.UpdatedAt, ex.Run.FinishedAt = status, kind, message, now, &now
	cleanup, cancel := cleanupContext(ctx)
	defer cancel()
	finishErr := e.repository.Finish(cleanup, ex.Run)
	if finishErr != nil {
		e.logger.Error("保存执行终态失败", "module", "run", "run_id", ex.Run.ID)
		ex.Run.Status, ex.Run.ErrorKind, ex.Run.ErrorMessage = run.Failed, new("internal"), new("保存执行状态失败")
		// 无法保存终态时只报告执行失败及持久化未确认，不能宣称数据库已进入终态。
		_ = e.emit(run.Event{Type: "run.failed", ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, Data: ex.Run, StatePersisted: new(false)})
		return
	} else if errors.Is(err, run.ErrStorage) {
		e.logger.Error("执行数据保存失败", "module", "run", "run_id", ex.Run.ID)
	}
	if finishErr == nil {
		if message, readErr := e.latestMessage(cleanup, ex); readErr == nil {
			_ = e.event(ex, "message.updated", "", message.ID, message)
		} else {
			e.logger.Error("读取最终消息状态失败", "module", "run", "run_id", ex.Run.ID)
		}
	}
	// 终态已持久化；发布器关闭后不递归发布错误，查询仍可获取已保存状态。
	_ = e.event(ex, "run."+ex.Run.Status, "", "", ex.Run)
}

func (e *Engine) initialEvents(ex conversation.Execution) error {
	if ex.Created {
		if err := e.event(ex, "conversation.created", "", "", ex.Conversation); err != nil {
			return err
		}
	} else {
		if err := e.event(ex, "conversation.updated", "", "", ex.Conversation); err != nil {
			return err
		}
	}
	if err := e.event(ex, "run.started", "", "", ex.Run); err != nil {
		return err
	}
	userEvent := "message.created"
	if ex.Run.ReplacesRunID != nil {
		userEvent = "message.updated"
	}
	if err := e.event(ex, userEvent, "", ex.User.ID, ex.User); err != nil {
		return err
	}
	if err := e.event(ex, "message.created", "", ex.Assistant.ID, ex.Assistant); err != nil {
		return err
	}
	return e.event(ex, "step.created", ex.Step.ID, "", ex.Step)
}

func (e *Engine) latestMessage(ctx context.Context, ex conversation.Execution) (conversation.Message, error) {
	rows, err := e.conversations.Messages(ctx, ex.Conversation.ID, 0, 1)
	if err != nil {
		return conversation.Message{}, err
	}
	if len(rows) != 1 || rows[0].ID != ex.Assistant.ID {
		return conversation.Message{}, conversation.ErrNotFound
	}
	return rows[0], nil
}

func (e *Engine) answer(ctx context.Context, lease *executor.Lease, ex conversation.Execution, resolved run.Resolved) (result model.Response, retErr error) {
	step := ex.Step
	now := time.Now().UnixMilli()
	step.Status, step.StartedAt, step.UpdatedAt = run.Running, &now, now
	request := model.Request{Model: resolved.Snapshot.RemoteModelID, CaptureResult: true, AllowImageInput: resolved.ImageInput}
	for _, message := range ex.History {
		request.Messages = append(request.Messages, model.Message{Role: model.Role(message.Role), Blocks: message.Content})
	}
	var text strings.Builder
	lastPersist := time.Now()
	persisted := 0
	defer func() {
		fillFailure(&result, resolved.Protocol, retErr)
		if len(result.Text) > inference.MaxTextBytes {
			retErr = errors.Join(retErr, run.ErrOutputLimit)
			result.Text = text.String()
			result.Content = nil
		}
		if result.Text == "" {
			result.Text = text.String()
		}
		if len(result.Content) == 0 && result.Text != "" {
			result.Content = []model.Block{{Type: "text", Text: result.Text}}
		}
		if result.Content == nil {
			result.Content = []model.Block{}
		}
		if cancelErr := lease.Finishing(); cancelErr != nil {
			retErr = errors.Join(cancelErr, retErr)
		}
		status := run.Completed
		if retErr != nil {
			var kind, message string
			status, kind, message = failure(retErr)
			step.ErrorKind, step.ErrorMessage = &kind, &message
		}
		now := time.Now().UnixMilli()
		step.Status, step.Output, step.Usage, step.UpdatedAt, step.FinishedAt = status, result.Output, result.Usage, now, &now
		if result.RequestID != "" {
			step.ProviderRequestID = &result.RequestID
		} else if call, ok := errors.AsType[*model.CallError](retErr); ok && call.RequestID != "" {
			step.ProviderRequestID = &call.RequestID
		}
		cleanup, cancel := cleanupContext(ctx)
		defer cancel()
		if err := e.repository.SaveStep(cleanup, step); err != nil {
			retErr = errors.Join(retErr, run.ErrStorage, err)
		} else {
			_ = e.event(ex, "step.updated", step.ID, "", step)
		}
		if err := e.repository.SaveMessage(cleanup, ex.Assistant.ID, result.Content, status); err != nil {
			retErr = errors.Join(retErr, run.ErrStorage, err)
		}
	}()
	if err := e.repository.SaveStep(ctx, step); err != nil {
		return result, errors.Join(run.ErrStorage, err)
	}
	if err := e.event(ex, "step.updated", step.ID, "", step); err != nil {
		return result, err
	}
	if ex.Run.Stream {
		retErr = resolved.Client.Stream(ctx, request, func(event model.Event) error {
			// 结果事件也可能在取消后交付，必须在检查取消前保留已取得的 usage。
			if event.Result != nil {
				result = *event.Result
				return nil
			}
			if event.RequestID != "" {
				result.RequestID = event.RequestID
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if event.Type != model.EventTextDelta {
				return nil
			}
			if text.Len()+len(event.Text) > inference.MaxTextBytes {
				return run.ErrOutputLimit
			}
			text.WriteString(event.Text)
			if persisted == 0 || text.Len()-persisted >= 4096 || time.Since(lastPersist) >= 100*time.Millisecond {
				if err := e.repository.SaveMessage(ctx, ex.Assistant.ID, []model.Block{{Type: "text", Text: text.String()}}, run.Running); err != nil {
					return errors.Join(run.ErrStorage, err)
				}
				lastPersist, persisted = time.Now(), text.Len()
			}
			return e.event(ex, "message.delta", step.ID, ex.Assistant.ID, map[string]any{"block_index": 0, "delta": map[string]any{"type": "text", "text": event.Text}})
		})
	} else {
		result, retErr = resolved.Client.Generate(ctx, request)
		if len(result.Text) > inference.MaxTextBytes {
			retErr = errors.Join(retErr, run.ErrOutputLimit)
			result.Text = ""
			result.Content = []model.Block{}
		}
	}
	return result, retErr
}
