package inference

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/model"
)

type fakeClient struct {
	generate func(context.Context, model.Request) (model.Response, error)
	stream   func(context.Context, model.Request, func(model.Event) error) error
}

func (c fakeClient) Generate(ctx context.Context, req model.Request) (model.Response, error) {
	return c.generate(ctx, req)
}
func (c fakeClient) Stream(ctx context.Context, req model.Request, emit func(model.Event) error) error {
	return c.stream(ctx, req, emit)
}

func nextEvent(t *testing.T, events <-chan Event) Event {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(3 * time.Second):
		t.Fatal("未收到执行事件")
		return Event{}
	}
}

func TestConcurrentExecutionCancellationAndCapacity(t *testing.T) {
	events := make(chan Event, 16)
	started := make(chan string, 2)
	finish := make(chan struct{})
	client := fakeClient{generate: func(ctx context.Context, req model.Request) (model.Response, error) {
		started <- req.Messages[0].Content
		select {
		case <-ctx.Done():
			return model.Response{}, ctx.Err()
		case <-finish:
			return model.Response{Text: req.Messages[0].Content}, nil
		}
	}}
	manager, err := New(t.Context(), func(context.Context, string) (model.Client, string, error) { return client, "remote", nil }, 2, time.Second, func(e Event) error { events <- e; return nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	var ids []string
	for _, text := range []string{"甲", "乙"} {
		in := Request{ModelID: "01900000-0000-7000-8000-000000000001", Messages: []model.Message{{Role: model.RoleUser, Content: text}}}
		if err := manager.Start(t.Context(), in, func(id string) error { ids = append(ids, id); return nil }); err != nil {
			t.Fatal(err)
		}
	}
	<-started
	<-started
	if err := manager.Start(t.Context(), Request{ModelID: "01900000-0000-7000-8000-000000000001", Messages: []model.Message{{Role: model.RoleUser}}}, func(string) error { return nil }); !errors.Is(err, ErrBusy) {
		t.Fatalf("并发上限未生效: %v", err)
	}
	if !manager.Cancel(ids[0]) {
		t.Fatal("取消未生效")
	}
	if event := nextEvent(t, events); event.RunID != ids[0] || event.Type != "cancelled" {
		t.Fatalf("取消事件错误: %+v", event)
	}
	close(finish)
	if event := nextEvent(t, events); event.RunID != ids[1] || event.Type != "completed" || event.Text == nil || *event.Text != "乙" {
		t.Fatalf("其他执行受到影响: %+v", event)
	}
	manager.Close()
	manager.Close()
	if manager.Cancel(ids[0]) {
		t.Fatal("已结束执行不应再次取消")
	}
	if err := manager.Start(t.Context(), Request{ModelID: "01900000-0000-7000-8000-000000000001", Messages: []model.Message{{Role: model.RoleUser}}}, func(string) error { return nil }); !errors.Is(err, ErrClosed) {
		t.Fatalf("关闭后仍接受执行: %v", err)
	}
}

func TestLookupContextDoesNotCancelAcceptedRun(t *testing.T) {
	lookupCtx, cancelLookup := context.WithCancel(t.Context())
	proceed := make(chan struct{})
	events := make(chan Event, 2)
	client := fakeClient{generate: func(ctx context.Context, req model.Request) (model.Response, error) {
		select {
		case <-proceed:
			return model.Response{Text: req.Messages[0].Content}, ctx.Err()
		case <-ctx.Done():
			return model.Response{}, ctx.Err()
		}
	}}
	manager, err := New(t.Context(), func(context.Context, string) (model.Client, string, error) { return client, "remote", nil }, 1, time.Second, func(e Event) error { events <- e; return nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	in := Request{ModelID: "01900000-0000-7000-8000-000000000001", Messages: []model.Message{{Role: model.RoleUser, Content: "原文"}}}
	if err := manager.Start(lookupCtx, in, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	cancelLookup()
	in.Messages[0].Content = "调用方已修改"
	close(proceed)
	event := nextEvent(t, events)
	if event.Type != "completed" || *event.Text != "原文" {
		t.Fatalf("上下文或消息快照隔离失败: %+v", event)
	}
}

func TestStreamTerminalStateUsesReturnValue(t *testing.T) {
	for _, test := range []struct {
		name   string
		stream func(context.Context, model.Request, func(model.Event) error) error
		want   string
		cause  error
	}{
		{"完整", func(_ context.Context, _ model.Request, emit func(model.Event) error) error {
			if err := emit(model.Event{Type: model.EventTextDelta, Text: "你好"}); err != nil {
				return err
			}
			return emit(model.Event{Type: model.EventCompleted})
		}, "completed", nil},
		{"完成回调后失败", func(_ context.Context, _ model.Request, emit func(model.Event) error) error {
			if err := emit(model.Event{Type: model.EventCompleted}); err != nil {
				return err
			}
			return model.ErrIncomplete
		}, "failed", model.ErrIncomplete},
		{"输出超限", func(_ context.Context, _ model.Request, emit func(model.Event) error) error {
			return emit(model.Event{Type: model.EventTextDelta, Text: strings.Repeat("x", MaxTextBytes+1)})
		}, "failed", ErrOutputLimit},
		{"超时", func(ctx context.Context, _ model.Request, _ func(model.Event) error) error {
			<-ctx.Done()
			return ctx.Err()
		}, "failed", context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			events := make(chan Event, 4)
			timeout := time.Second
			if errors.Is(test.cause, context.DeadlineExceeded) {
				timeout = 50 * time.Millisecond
			}
			manager, err := New(t.Context(), func(context.Context, string) (model.Client, string, error) {
				return fakeClient{stream: test.stream}, "remote", nil
			}, 1, timeout, func(e Event) error { events <- e; return nil })
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(manager.Close)
			if err := manager.Start(t.Context(), Request{ModelID: "01900000-0000-7000-8000-000000000001", Stream: true, Messages: []model.Message{{Role: model.RoleUser}}}, func(string) error { return nil }); err != nil {
				t.Fatal(err)
			}
			managerEvent := nextEvent(t, events)
			if managerEvent.Type == "text_delta" {
				managerEvent = nextEvent(t, events)
			}
			if managerEvent.Type != test.want || !errors.Is(managerEvent.Err, test.cause) {
				t.Fatalf("终态错误: %+v", managerEvent)
			}
			manager.Close()
			if len(events) != 0 {
				t.Fatal("产生了重复终态")
			}
		})
	}
}

func TestRejectedAcceptanceDoesNotCallModelOrConsumeCapacity(t *testing.T) {
	called := make(chan struct{}, 1)
	events := make(chan Event, 1)
	client := fakeClient{generate: func(context.Context, model.Request) (model.Response, error) {
		called <- struct{}{}
		return model.Response{}, nil
	}}
	manager, err := New(t.Context(), func(context.Context, string) (model.Client, string, error) { return client, "remote", nil }, 1, time.Second, func(e Event) error { events <- e; return nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	in := Request{ModelID: "01900000-0000-7000-8000-000000000001", Messages: []model.Message{{Role: model.RoleUser}}}
	rejected := errors.New("响应无法写入")
	if err := manager.Start(t.Context(), in, func(string) error { return rejected }); !errors.Is(err, rejected) {
		t.Fatal(err)
	}
	if len(called) != 0 {
		t.Fatal("接受失败仍然调用了模型")
	}
	if err := manager.Start(t.Context(), in, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	nextEvent(t, events)
}
