package app

import (
	"context"
	"sync"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/executor"
)

// sessionRuns 只登记本连接发起的执行；登记发生在受理响应之前。
// 已完成标识在下次登记时清理，集合大小不超过调度容量，不为每次执行启动等待任务。
type sessionRuns struct {
	ctx     context.Context
	manager *executor.Manager
	mu      sync.Mutex
	ids     map[string]struct{}
	closed  bool
}

func newSessionRuns(ctx context.Context, manager *executor.Manager) *sessionRuns {
	return &sessionRuns{ctx: ctx, manager: manager, ids: make(map[string]struct{})}
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
	for previous := range s.ids {
		if !s.manager.Active(previous) {
			delete(s.ids, previous)
		}
	}
	s.ids[id] = struct{}{}
	return nil
}

func (s *sessionRuns) Close() {
	s.mu.Lock()
	s.closed = true
	ids := make([]string, 0, len(s.ids))
	for id := range s.ids {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		s.manager.Cancel(id)
	}
	for _, id := range ids {
		s.manager.Wait(id)
	}
}
