package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/events"
	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/provider"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

func runtimeFixture(t *testing.T, limit int) (*Runtime, config.Config) {
	t.Helper()
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConcurrentRuns = limit
	runtime, err := New(t.Context(), cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Error(err)
		}
	})
	return runtime, cfg
}

func directModel(t *testing.T, runtime *Runtime, url string) string {
	t.Helper()
	pid, err := runtime.Providers().CreateProvider(t.Context(), provider.ProviderInput{Name: "厂商", Protocol: "chatcompletions", BaseURL: url + "/v1", APIKey: new("fake-runtime-key")})
	if err != nil {
		t.Fatal(err)
	}
	id, err := runtime.Providers().CreateModel(t.Context(), provider.ModelInput{ProviderID: pid, DisplayName: "模型", RemoteModelID: "remote", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func nextBusiness(t *testing.T, sub *events.Subscription, kind, id string) agent.Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	for {
		event, err := sub.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if event.Type == kind && (id == "" || event.RunID == id) {
			return event
		}
	}
}

func sharedPeer(t *testing.T, runtime *Runtime) *appPeer {
	t.Helper()
	server, client := net.Pipe()
	ctx, cancel := context.WithCancel(t.Context())
	p := &appPeer{conn: client, decoder: json.NewDecoder(client), done: make(chan error, 1), cancel: cancel}
	go func() { p.done <- runtime.ServeStdio(ctx, server, server) }()
	t.Cleanup(func() {
		cancel()
		client.Close()
		if !p.stopped {
			select {
			case err := <-p.done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(3 * time.Second):
				t.Error("共享连接未退出")
			}
		}
	})
	if event := p.read(t); event.Method != "system.ready" {
		t.Fatalf("首个消息不是就绪事件: %+v", event)
	}
	return p
}

func TestRuntimeWithoutProtocol(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"业务回答"},"finish_reason":"stop"}],"usage":{"total_tokens":5}}`)
	}))
	t.Cleanup(upstream.Close)
	runtime, _ := runtimeFixture(t, 1)
	id := directModel(t, runtime, upstream.URL)
	first, err := runtime.Subscribe(events.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, _ := runtime.Subscribe(events.Filter{})
	var accepted conversation.Execution
	err = runtime.Conversations().Send(t.Context(), conversation.Send{ModelID: id, Stream: new(false), Content: []model.Block{{Type: "text", Text: "问题"}}}, func(ex conversation.Execution) error {
		accepted = ex
		// 取消一个观察者不影响发送受理及另一个观察者。
		second.Close()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	nextBusiness(t, first, "run.completed", accepted.Run.ID)
	saved, err := runtime.Runs().Get(t.Context(), accepted.Run.ID)
	if err != nil || saved.Status != agent.Completed {
		t.Fatalf("直接调用未持久化: %+v %v", saved, err)
	}
	step, err := runtime.Runs().GetStep(t.Context(), accepted.Step.ID)
	if err != nil || len(step.Output) == 0 {
		t.Fatalf("完整结果无法查询: %+v %v", step, err)
	}
	title := "手动标题"
	_, err = runtime.Conversations().Update(t.Context(), accepted.Conversation.ID, conversation.Change{Title: &title}, "")
	if err != nil {
		t.Fatal(err)
	}
	event := nextBusiness(t, first, "conversation.updated", "")
	value, ok := event.Data.(conversation.Conversation)
	if !ok || value.Title != title || value.TitleSource != "manual" {
		t.Fatalf("直接业务修改没有发布事件: %+v", event)
	}
}

func TestSharedRuntimeConnectionCleanup(t *testing.T) {
	ownedStarted, otherStarted := make(chan struct{}), make(chan struct{})
	releaseOther := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Messages) == 0 {
			t.Error("模型请求无效")
			return
		}
		text := input.Messages[len(input.Messages)-1].Content
		if input.Messages[0].Role == "system" {
			text = "title"
		}
		switch text {
		case "owned":
			close(ownedStarted)
			<-r.Context().Done()
			return
		case "other":
			close(otherStarted)
			select {
			case <-releaseOther:
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"回答"},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(upstream.Close)
	runtime, _ := runtimeFixture(t, 2)
	id := directModel(t, runtime, upstream.URL)
	p := sharedPeer(t, runtime)
	accepted := p.call(t, "conversation.send", map[string]any{"model_id": id, "stream": false, "content": messageBlocks("owned")})
	ownedID := resultString(t, accepted, "run_id")
	waitSignal(t, ownedStarted)
	sub, err := runtime.Subscribe(events.Filter{Types: []string{"run.completed", "run.cancelled"}})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	var otherID string
	if err := runtime.Invoke(t.Context(), inference.Request{ModelID: id, Messages: []model.Message{{Role: model.RoleUser, Content: "other"}}}, func(id string) error { otherID = id; return nil }); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, otherStarted)
	requireOK(t, p.call(t, "system.shutdown", map[string]any{}))
	p.event(t, "run.cancelled")
	select {
	case err := <-p.done:
		p.stopped = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("连接清理超时")
	}
	if runtime.manager.Active(ownedID) || !runtime.manager.Active(otherID) {
		t.Fatal("连接清理影响了其他调用方")
	}
	close(releaseOther)
	nextBusiness(t, sub, "run.completed", otherID)
	if _, err := runtime.Providers().GetModel(t.Context(), id); err != nil {
		t.Fatalf("连接退出后数据库被关闭: %v", err)
	}
	// 一个新连接仍可复用同一运行时。
	next := sharedPeer(t, runtime)
	next.stop(t)
}

func waitSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatal("未观察到模型请求")
	}
}

func TestRuntimeCloseCancelsAndReleasesOwnership(t *testing.T) {
	started := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			return
		}
		close(started)
		<-r.Context().Done()
	}))
	t.Cleanup(upstream.Close)
	runtime, cfg := runtimeFixture(t, 1)
	id := directModel(t, runtime, upstream.URL)
	peer := sharedPeer(t, runtime)
	if err := runtime.Invoke(t.Context(), inference.Request{ModelID: id, Messages: []model.Message{{Role: model.RoleUser, Content: "wait"}}}, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, started)
	// 第二次初始化必须在取得实例锁时失败，不能触碰活动实例的数据。
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	if duplicate, err := New(t.Context(), cfg, logger); err == nil {
		duplicate.Close()
		t.Fatal("重复运行时取得了实例锁")
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-peer.done:
		peer.stopped = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("应用关闭未等待连接退出")
	}
	if err := runtime.Invoke(t.Context(), inference.Request{ModelID: id, Messages: []model.Message{{Role: model.RoleUser, Content: "late"}}}, func(string) error { return nil }); !errors.Is(err, agent.ErrClosed) {
		t.Fatalf("关闭后仍接受执行: %v", err)
	}
	recovered, err := New(t.Context(), cfg, logger)
	if err != nil {
		t.Fatalf("实例锁未释放: %v", err)
	}
	if err := recovered.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCancelAcceptedBeforeModelStarts(t *testing.T) {
	runtime, _ := runtimeFixture(t, 1)
	id := directModel(t, runtime, "https://example.test")
	subscription, err := runtime.Subscribe(events.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Close()
	var runID string
	err = runtime.Conversations().Send(t.Context(), conversation.Send{ModelID: id, Content: []model.Block{{Type: "text", Text: "立即取消"}}}, func(ex conversation.Execution) error {
		runID = ex.Run.ID
		if !runtime.Cancel(runID) {
			t.Error("受理阶段不能取消执行")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	nextBusiness(t, subscription, "run.cancelled", runID)
	value, err := runtime.Runs().Get(t.Context(), runID)
	if err != nil || value.Status != agent.Cancelled {
		t.Fatalf("受理后的取消被当作存储失败: %+v %v", value, err)
	}
}

func TestOwnershipPrecedesMigration(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(t.Context(), cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, sql := range []string{"DROP TABLE app_settings", "DROP TABLE conversation_run_approvals", "DROP INDEX uq_step_run", "PRAGMA user_version=2"} {
		if err := db.DB.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	ownership, err := sqlite.AcquireOwnership(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer ownership.Close()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	if duplicate, err := New(t.Context(), cfg, logger); err == nil {
		duplicate.Close()
		t.Fatal("重复实例取得了锁")
	}
	var version int
	if err := db.DB.Raw("PRAGMA user_version").Scan(&version).Error; err != nil {
		t.Fatal(err)
	}
	if version != 2 || db.DB.Migrator().HasTable("conversation_run_approvals") || db.DB.Migrator().HasTable("app_settings") {
		t.Fatal("未取得锁就迁移了活动实例的数据")
	}
	if err := ownership.Close(); err != nil {
		t.Fatal(err)
	}
	runtime, err := New(t.Context(), cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
}
