// Package approval 管理确定操作的审批，等待过程不占用事务或模型请求槽位。
package approval

import (
	"context"
	"errors"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/xiaoyu98628/callum/internal/agent"
)

// Service 由应用持有，等待与所属执行共享取消信号，不启动独立后台任务。
type Service struct {
	repository agent.ApprovalRepository
	lookup     func(context.Context, string) (agent.Run, error)
	emit       func(agent.Event) error
	mu         sync.Mutex
	signals    map[string]chan struct{}
}

func New(repository agent.ApprovalRepository, lookup func(context.Context, string) (agent.Run, error), emit func(agent.Event) error) (*Service, error) {
	if repository == nil || lookup == nil || emit == nil {
		return nil, agent.ErrApprovalInvalid
	}
	return &Service{repository: repository, lookup: lookup, emit: emit, signals: map[string]chan struct{}{}}, nil
}
func validID(id string) bool {
	value, err := uuid.Parse(id)
	return err == nil && value.Version() == 7 && value.String() == id
}

func (s *Service) Get(ctx context.Context, id string) (agent.Approval, error) {
	if !validID(id) {
		return agent.Approval{}, agent.ErrApprovalInvalid
	}
	return s.repository.GetApproval(ctx, id)
}
func (s *Service) List(ctx context.Context, f agent.ApprovalFilter) ([]agent.Approval, error) {
	if f.Limit == 0 {
		f.Limit = 100
	}
	if f.Limit < 1 || f.Limit > 200 || f.Offset < 0 || (f.RunID != "" && !validID(f.RunID)) {
		return nil, agent.ErrApprovalInvalid
	}
	switch f.Status {
	case "", agent.ApprovalPending, agent.ApprovalApproved, agent.ApprovalRejected, agent.ApprovalCancelled:
	default:
		return nil, agent.ErrApprovalInvalid
	}
	return s.repository.ListApprovals(ctx, f)
}

// Decide 的用户标识由本机服务确定，客户端仅能提交决定和说明。
func (s *Service) Decide(ctx context.Context, id, decision string, note *string) (agent.Approval, error) {
	if !validID(id) || (decision != agent.ApprovalApproved && decision != agent.ApprovalRejected) || (note != nil && (len(*note) > 4096 || !utf8.ValidString(*note))) {
		return agent.Approval{}, agent.ErrApprovalInvalid
	}
	value, err := s.repository.DecideApproval(ctx, id, decision, new("local_user"), note)
	if value.ID != "" && err == nil {
		s.signal(value.ID)
		if publishErr := s.publish(ctx, "approval.updated", value); publishErr != nil {
			return value, errors.Join(err, publishErr)
		}
	}
	return value, err
}

func (s *Service) signal(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ch := s.signals[id]; ch != nil {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
func (s *Service) publish(ctx context.Context, name string, a agent.Approval) error {
	execution, err := s.lookup(ctx, a.RunID)
	if err != nil {
		return err
	}
	return s.emit(agent.Event{Type: name, ConversationID: execution.ConversationID, RunID: a.RunID, StepID: a.StepID, Data: a})
}

// Request 在落库后才发布审批通知，审批快照和哈希必须由执行器生成。
func (s *Service) Request(ctx context.Context, runID, stepID string, request []byte, hash string) (agent.Approval, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return agent.Approval{}, err
	}
	now := time.Now()
	value, err := s.repository.CreateApproval(ctx, agent.Approval{ID: id.String(), RunID: runID, StepID: stepID, Request: request, RequestHash: hash, Status: agent.ApprovalPending, CreatedAt: now.UnixMilli(), UpdatedAt: now.UnixMilli()})
	if err != nil {
		return value, err
	}
	return value, s.publish(ctx, "approval.requested", value)
}

// Wait 先登记唤醒通道再读取记录，避免决定先于等待导致丢失唤醒。
// 通道数量由活动执行限制，退出时立即删除；取消后以短清理上下文保存状态。
func (s *Service) Wait(ctx context.Context, id string) (agent.Approval, error) {
	ch := make(chan struct{}, 1)
	s.mu.Lock()
	if s.signals[id] != nil {
		s.mu.Unlock()
		return agent.Approval{}, agent.ErrConflict
	}
	s.signals[id] = ch
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.signals, id); s.mu.Unlock() }()
	for {
		if err := ctx.Err(); err != nil {
			return s.cancelPending(ctx, id, err)
		}
		value, err := s.repository.GetApproval(ctx, id)
		if err != nil {
			return value, err
		}
		if value.Status != agent.ApprovalPending {
			return value, nil
		}
		// 等待由用户决定或所属执行的取消信号结束，不设置审批期限。
		select {
		case <-ctx.Done():
			return s.cancelPending(ctx, id, ctx.Err())
		case <-ch:
		}
	}
}
func (s *Service) cancelPending(ctx context.Context, id string, cause error) (agent.Approval, error) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	value, err := s.repository.CancelApproval(cleanup, id)
	if err == nil {
		err = s.publish(cleanup, "approval.updated", value)
	}
	return value, errors.Join(cause, err)
}
