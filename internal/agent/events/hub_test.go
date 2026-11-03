package events

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/settings"
)

func TestSubscriptionIsolationAndBounds(t *testing.T) {
	hub, err := New(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Close()
	fast, _ := hub.Subscribe(Filter{ConversationID: "a", Types: []string{"message.delta"}})
	slow, _ := hub.Subscribe(Filter{})
	if _, err := hub.Subscribe(Filter{}); !errors.Is(err, ErrSubscriberLimit) {
		t.Fatalf("订阅上限: %v", err)
	}
	// 过滤掉的事件不能占用 fast 容量；slow 第二次发布时溢出。
	if err := hub.Publish(agent.Event{Type: "run.started", ConversationID: "b"}); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		event := agent.Event{Type: "message.delta", ConversationID: "a", Data: i}
		if err := hub.Publish(event); err != nil {
			t.Fatal(err)
		}
		got, err := fast.Next(t.Context())
		if err != nil || got.Data != i {
			t.Fatalf("快速订阅受到影响: %+v %v", got, err)
		}
	}
	if _, err := slow.Next(t.Context()); !errors.Is(err, ErrSlowConsumer) {
		t.Fatalf("未报告溢出: %v", err)
	}
	replacement, err := hub.Subscribe(Filter{})
	if err != nil {
		t.Fatal(err)
	}
	replacement.Close()
	fast.Close()
	fast.Close()
	if _, err := fast.Next(t.Context()); !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	// 无订阅者时不阻塞，也不使执行失败。
	if err := hub.Publish(agent.Event{}); err != nil {
		t.Fatal(err)
	}
	hub.Close()
	if err := hub.Publish(agent.Event{}); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if _, err := hub.Subscribe(Filter{}); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}

func TestRunSettingsDoNotShareExecutionPointer(t *testing.T) {
	hub, err := New(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Close()
	sub, err := hub.Subscribe(Filter{})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	snapshot := &settings.Snapshot{Revision: 3, Execution: settings.DefaultExecution()}
	if err := hub.Publish(agent.Event{Type: "run.started", Data: agent.Run{SettingsSnapshot: snapshot}}); err != nil {
		t.Fatal(err)
	}
	snapshot.Revision = 4
	event, err := sub.Next(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	run := event.Data.(agent.Run)
	if run.SettingsSnapshot.Revision != 3 {
		t.Fatal("队列借用了执行内部的快照")
	}
	run.SettingsSnapshot.ApprovalPolicy = "never"
	if snapshot.ApprovalPolicy != "on-request" {
		t.Fatal("事件消费者改变了执行内部权限")
	}
}

func TestSummaryAndDrain(t *testing.T) {
	hub, _ := New(2, 1)
	sub, _ := hub.Subscribe(Filter{RunID: "r"})
	step := agent.Step{ID: "s", Output: []byte(`{"large":true}`), Usage: []byte(`{"total_tokens":3}`), Definition: []byte(`{}`)}
	now := int64(1)
	step.StartedAt = &now
	if err := hub.Publish(agent.Event{Type: "step.updated", RunID: "r", Data: step}); err != nil {
		t.Fatal(err)
	}
	now = 2
	hub.Close()
	event, err := sub.Next(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	data, ok := event.Data.(agent.StepSummary)
	if !ok || data.Output != nil || data.Usage != nil || data.Definition != nil || data.OutputBytes != len(step.Output) || data.UsageBytes != len(step.Usage) {
		t.Fatalf("队列未投影步骤: %+v", data)
	}
	if data.StartedAt == nil || *data.StartedAt != 1 {
		t.Fatal("已发布快照被后续更新改写")
	}
	if _, err := sub.Next(t.Context()); !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
}

func TestConcurrentPublishAndClose(t *testing.T) {
	hub, _ := New(4, 8)
	sub, _ := hub.Subscribe(Filter{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := sub.Next(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				_ = hub.Publish(agent.Event{Type: "message.delta"})
				next, err := hub.Subscribe(Filter{})
				if err == nil {
					next.Close()
				}
			}
		})
	}
	wg.Go(func() { sub.Close(); hub.Close() })
	wg.Wait()
}
