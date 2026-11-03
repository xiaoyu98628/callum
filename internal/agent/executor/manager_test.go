package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
)

func TestTerminalDecisionAndAdmissionAfterCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	manager, err := New(ctx, 1, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	lease, err := manager.Reserve("first", "conversation")
	if err != nil {
		t.Fatal(err)
	}
	finishing, release := make(chan struct{}), make(chan struct{})
	lease.Start(func(ctx context.Context) {
		if err := lease.Finishing(); err != nil {
			t.Error(err)
		}
		close(finishing)
		select {
		case <-release:
		case <-ctx.Done():
		}
	})
	<-finishing
	if manager.Cancel("first") {
		t.Error("已确定终态仍允许取消")
	}
	returned := make(chan *Lease, 1)
	failures := make(chan error, 1)
	go func() {
		next, err := manager.ReserveContext(t.Context(), "second", "conversation")
		if err != nil {
			failures <- err
			return
		}
		returned <- next
	}()
	close(release)
	select {
	case next := <-returned:
		next.Abort()
	case err := <-failures:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("终态任务清理后未释放容量")
	}
	cancel()
	if err := lease.Finishing(); err != nil {
		t.Fatalf("关闭上下文改写了已确定终态: %v", err)
	}
	manager.Close()
	manager.Close()
	if err := manager.Call(t.Context(), func() error { t.Error("关闭后调用模型"); return nil }); !errors.Is(err, agent.ErrClosed) {
		t.Fatalf("关闭后未拒绝调用: %v", err)
	}
}

func TestModelCallsShareLimitAndAuxiliaryDoesNotQueue(t *testing.T) {
	manager, err := New(t.Context(), 1, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- manager.Call(t.Context(), func() error {
			close(started)
			select {
			case <-release:
			case <-t.Context().Done():
			}
			return nil
		})
	}()
	<-started
	if acquired, err := manager.TryCall(t.Context(), func() error { t.Error("辅助调用绕过上限"); return nil }); acquired || err != nil {
		t.Fatalf("辅助调用不应排队: %v %v", acquired, err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	first, err := manager.Reserve("first", "same")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Reserve("second", "same"); !errors.Is(err, agent.ErrBusy) {
		t.Fatal("未限制同会话")
	}
	first.Abort()
	second, err := manager.Reserve("second", "same")
	if err != nil {
		t.Fatal(err)
	}
	second.Abort()
}
