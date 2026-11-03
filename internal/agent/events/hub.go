// Package events 提供运行时内的有界事件订阅，不负责持久化或协议编码。
package events

import (
	"context"
	"errors"
	"io"
	"slices"
	"sync"

	"github.com/xiaoyu98628/callum/internal/agent"
)

var (
	ErrClosed          = errors.New("事件发布器已关闭")
	ErrSlowConsumer    = errors.New("事件订阅队列已满")
	ErrSubscriberLimit = errors.New("事件订阅数量已达到上限")
)

// Filter 的非空条件同时生效；空过滤器订阅所有事件。
type Filter struct {
	ConversationID string
	RunID          string
	Types          []string
}

// Hub 由应用运行时持有并关闭，不启动后台任务。
type Hub struct {
	mu              sync.Mutex
	closed          bool
	capacity, limit int
	subs            map[*Subscription]struct{}
}

// Subscription 只允许一个 Next 调用方；持有者负责 Close，事件内容只读。
type Subscription struct {
	hub    *Hub
	filter Filter
	queue  chan agent.Event
	closed bool
	cause  error
}

// New 的容量和订阅数量必须为正数，限制由装配层确定。
func New(capacity, limit int) (*Hub, error) {
	if capacity < 1 || limit < 1 {
		return nil, agent.ErrInvalid
	}
	return &Hub{capacity: capacity, limit: limit, subs: make(map[*Subscription]struct{})}, nil
}

func (h *Hub) Subscribe(filter Filter) (*Subscription, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, ErrClosed
	}
	if len(h.subs) >= h.limit {
		return nil, ErrSubscriberLimit
	}
	filter.Types = slices.Clone(filter.Types)
	s := &Subscription{hub: h, filter: filter, queue: make(chan agent.Event, h.capacity)}
	h.subs[s] = struct{}{}
	return s, nil
}

// Publish 不等待消费者；队列溢出只断开对应订阅，不使业务执行失败。
// 同次发布的内容由订阅者共享，发布方与订阅者均不得修改事件引用的数据。
func (h *Hub) Publish(event agent.Event) error {
	if step, ok := event.Data.(agent.Step); ok {
		event.Data = agent.SummarizeStep(step)
	}
	if run, ok := event.Data.(agent.Run); ok && run.SettingsSnapshot != nil {
		// 消费者遵守只读约定，发布器同时避免队列引用执行器内部的权限指针。
		snapshot := *run.SettingsSnapshot
		run.SettingsSnapshot = &snapshot
		event.Data = run
	}
	event.RemovedMessageIDs = slices.Clone(event.RemovedMessageIDs)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return ErrClosed
	}
	for s := range h.subs {
		f := s.filter
		if (f.ConversationID != "" && f.ConversationID != event.ConversationID) || (f.RunID != "" && f.RunID != event.RunID) || (len(f.Types) > 0 && !slices.Contains(f.Types, event.Type)) {
			continue
		}
		select {
		case s.queue <- event:
		default:
			s.closeLocked(ErrSlowConsumer)
		}
	}
	return nil
}

// Next 返回排队事件；正常关闭后排空队列再返回 EOF，溢出立即返回错误。
func (s *Subscription) Next(ctx context.Context) (agent.Event, error) {
	select {
	case <-ctx.Done():
		return agent.Event{}, ctx.Err()
	case event, ok := <-s.queue:
		s.hub.mu.Lock()
		cause := s.cause
		s.hub.mu.Unlock()
		if cause != nil {
			return agent.Event{}, cause
		}
		if !ok {
			return agent.Event{}, io.EOF
		}
		return event, nil
	}
}

func (s *Subscription) closeLocked(cause error) {
	if s.closed {
		return
	}
	s.closed, s.cause = true, cause
	delete(s.hub.subs, s)
	close(s.queue)
	if cause != nil {
		for range s.queue {
		}
	}
}

// Close 解除订阅，可重复调用；已排队的事件仍可读取。
func (s *Subscription) Close() {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	s.closeLocked(nil)
}

// Close 停止发布并关闭全部订阅，应在业务生产者停止后调用。
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for s := range h.subs {
		s.closeLocked(nil)
	}
}
