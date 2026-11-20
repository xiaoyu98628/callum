package app

import (
	"context"
	"sync"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/executor"
)

// sessionRuns 只登记本连接发起的执行；登记发生在受理响应之前。
// 连接持有本次受理的租约。使用同一 ID 继续的另一个连接拥有独立租约。
type sessionRuns struct {
	ctx     context.Context
	manager *executor.Manager
	mu      sync.Mutex
	leases  map[string]*executor.Lease
	closed  bool
}

func newSessionRuns(ctx context.Context, manager *executor.Manager) *sessionRuns {
	return &sessionRuns{ctx: ctx, manager: manager, leases: make(map[string]*executor.Lease)}
}

func (s *sessionRuns) Register(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return agent.ErrClosed
	}
	if err := s.ctx.Err(); err != nil {
		return err
	}
	lease := s.manager.ActiveLease(id)
	if lease == nil {
		return agent.ErrNotFound
	}
	for previous, owned := range s.leases {
		if s.manager.ActiveLease(previous) != owned {
			delete(s.leases, previous)
		}
	}
	s.leases[id] = lease
	return nil
}

func (s *sessionRuns) Close() {
	s.mu.Lock()
	s.closed = true
	leases := make([]*executor.Lease, 0, len(s.leases))
	for _, lease := range s.leases {
		leases = append(leases, lease)
	}
	s.mu.Unlock()
	for _, lease := range leases {
		lease.Cancel()
	}
	for _, lease := range leases {
		lease.Wait()
	}
}
