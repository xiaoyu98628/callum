package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/executor"
	"github.com/xiaoyu98628/callum/internal/conversation"
)

type emptySteeringRepository struct{ conversation.Repository }

func (emptySteeringRepository) PendingSteering(context.Context, string) ([]conversation.FollowUp, error) {
	return nil, nil
}

func steeringEngine(t *testing.T) *Engine {
	t.Helper()
	manager, err := executor.New(t.Context(), 2, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	return &Engine{manager: manager, conversations: emptySteeringRepository{},
		steering: make(map[*executor.Lease]*runSteering), emit: func(agent.Event) error { return nil }}
}

func reserveSteering(t *testing.T, e *Engine, runID, conversationID string) (*executor.Lease, *runSteering) {
	t.Helper()
	lease, err := e.manager.ReserveAgentContext(t.Context(), runID, conversationID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(lease.Abort)
	return lease, e.registerSteering(lease, runID)
}

func TestSteeringCommitDoesNotBlockAnotherExecution(t *testing.T) {
	e := steeringEngine(t)
	first, firstState := reserveSteering(t, e, "first", "conversation-a")
	_, otherState := reserveSteering(t, e, "second", "conversation-b")
	sampling, finish := firstState.samplingContext(first.Context(), 0)
	defer finish()

	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	committed := make(chan error, 1)
	go func() {
		committed <- e.AcceptSteering(first, func() (conversation.FollowUp, error) {
			close(entered)
			<-release
			return conversation.FollowUp{ConversationID: "conversation-a"}, nil
		})
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("引导未进入提交边界")
	}
	progress := make(chan error, 1)
	go func() {
		_, _, err := e.snapshotSteering(ctx, otherState)
		if err == nil {
			err = e.beforeTool(ctx, otherState, func() error { return nil })
		}
		progress <- err
	}()
	select {
	case err := <-progress:
		if err != nil || sampling.Err() != nil {
			t.Fatalf("另一会话推进失败或引导在提交前取消采样: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("会话 A 提交期间阻塞了会话 B")
	}
	unblock()
	if err := <-committed; err != nil || !errors.Is(context.Cause(sampling), conversation.ErrSteered) || first.Context().Err() != nil {
		t.Fatalf("引导提交未按本次采样边界生效: %v", err)
	}
}

func TestSteeringRejectsLeaseFromPreviousExecution(t *testing.T) {
	e := steeringEngine(t)
	old, state := reserveSteering(t, e, "same", "conversation")
	if e.SteeringLease("same") != old {
		t.Fatal("未找到本次执行的引导租约")
	}
	e.sealSteering(t.Context(), state)
	staleWrite := func() (conversation.FollowUp, error) {
		t.Fatal("旧租约仍可提交引导")
		return conversation.FollowUp{}, nil
	}
	if err := e.AcceptSteering(old, staleWrite); !errors.Is(err, conversation.ErrFollowUpChanged) {
		t.Fatalf("封存后仍接受引导: %v", err)
	}
	e.removeSteering(old)
	old.Abort()

	current, currentState := reserveSteering(t, e, "same", "conversation")
	if current == old || e.SteeringLease("same") != current {
		t.Fatal("原地恢复未建立独立引导租约")
	}
	sampling, finish := currentState.samplingContext(current.Context(), 0)
	defer finish()
	if err := e.AcceptSteering(old, staleWrite); !errors.Is(err, conversation.ErrFollowUpChanged) || sampling.Err() != nil {
		t.Fatalf("旧租约影响了新的模型采样: %v", err)
	}
	if err := e.AcceptSteering(current, func() (conversation.FollowUp, error) {
		return conversation.FollowUp{ConversationID: "conversation"}, nil
	}); err != nil || !errors.Is(context.Cause(sampling), conversation.ErrSteered) {
		t.Fatalf("当前租约的引导未生效: %v", err)
	}
}
