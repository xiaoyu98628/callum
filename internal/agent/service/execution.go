package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/executor"
	"github.com/xiaoyu98628/callum/internal/conversation"
)

// Execute 只在受理响应已排队后运行；标题任务在终态前取消并等待退出。
func (e *Engine) Execute(ctx context.Context, lease *executor.Lease, ex conversation.Execution, resolved agent.Resolved) {
	err := e.initialEvents(ex)
	if ex.Run.SettingsSnapshot == nil || !ex.Run.SettingsSnapshot.Valid() {
		err = errors.Join(err, agent.ErrInvalid)
	}
	var titleWG sync.WaitGroup
	titleCtx, titleCancel := context.WithCancel(ctx)
	if err == nil {
		err = e.loop(ctx, ex, resolved, func() {
			if ex.User.Sequence == 1 && ex.Conversation.TitleSource == "default" {
				if release, ok, _ := e.manager.TryAcquireCall(titleCtx); ok {
					titleWG.Go(func() { defer release(); e.title(titleCtx, ex, resolved) })
				}
			}
		})
	}
	titleCancel()
	titleWG.Wait()
	if cancelErr := lease.Finishing(); cancelErr != nil {
		err = errors.Join(cancelErr, err)
	}
	status := agent.Completed
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
		ex.Run.Status, ex.Run.ErrorKind, ex.Run.ErrorMessage = agent.Failed, new("internal"), new("保存执行状态失败")
		// 无法保存终态时只报告执行失败及持久化未确认，不能宣称数据库已进入终态。
		_ = e.emit(agent.Event{Type: "run.failed", ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, Data: ex.Run, StatePersisted: new(false)})
		return
	} else if errors.Is(err, agent.ErrStorage) {
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
