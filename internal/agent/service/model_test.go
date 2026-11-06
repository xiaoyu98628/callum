package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/executor"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/model"
)

type interruptedMessageRepository struct {
	agent.Repository
	write        func(context.Context) error
	cleanupError error
}

func (r *interruptedMessageRepository) SaveMessage(ctx context.Context, _ string, _ []model.Block, _ string) error {
	return r.write(ctx)
}

func (r *interruptedMessageRepository) SaveStep(_ context.Context, step agent.Step) error {
	if step.Status != agent.Running {
		return r.cleanupError
	}
	return nil
}

type preambleStream struct{ model.Client }

func (preambleStream) Stream(_ context.Context, _ model.Request, emit func(model.Event) error) error {
	return emit(model.Event{Type: model.EventTextDelta, Text: "准备检查项目。"})
}

func TestCancelledIncrementalSaveDoesNotBecomeStorageFailure(t *testing.T) {
	writeFailure := errors.New("controlled storage failure")
	for _, test := range []struct {
		name                           string
		cancel, failWrite, failCleanup bool
		wantStatus                     string
		wantStorage                    bool
	}{
		{"cancelled_write", true, false, false, agent.Cancelled, false},
		{"real_write_failure", false, true, false, agent.Failed, true},
		{"real_write_failure_with_cancel", true, true, false, agent.Failed, true},
		{"cancelled_write_with_cleanup_failure", true, false, true, agent.Failed, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			writes := 0
			repository := &interruptedMessageRepository{write: func(ctx context.Context) error {
				writes++
				// 确定性地让取消发生在保存正文时，不依赖网络与数据库的调度时机。
				if test.cancel {
					cancel()
				}
				if test.failWrite {
					return writeFailure
				}
				return ctx.Err()
			}}
			if test.failCleanup {
				repository.cleanupError = writeFailure
			}
			manager, err := executor.New(t.Context(), 1, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer manager.Close()
			engine := &Engine{repository: repository, manager: manager, emit: func(agent.Event) error { return nil }, options: Options{CallTimeout: time.Second}}
			ex := conversation.Execution{Run: agent.Run{Stream: true}, Assistant: conversation.Message{ID: "assistant"}}
			resolved := agent.Resolved{Client: preambleStream{}}
			total := 0
			_, err = engine.callModel(ctx, ex, resolved, agent.Step{Input: []byte(`{}`)}, nil, nil, func() {}, &total)
			status, _, _ := failure(err)
			if writes != 1 || status != test.wantStatus || errors.Is(err, agent.ErrStorage) != test.wantStorage {
				t.Fatalf("增量保存错误分类不符: writes=%d status=%s err=%v", writes, status, err)
			}
			if test.cancel && !errors.Is(err, context.Canceled) {
				t.Fatal("取消原因丢失")
			}
			if (test.failWrite || test.failCleanup) && !errors.Is(err, writeFailure) {
				t.Fatal("真实存储错误链丢失")
			}
		})
	}
}
