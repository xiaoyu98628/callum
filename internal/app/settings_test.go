package app

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/events"
	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

func readSettings(t *testing.T, p *appPeer) settings.Record {
	t.Helper()
	frame := p.call(t, "settings.get", map[string]any{"key": "execution"})
	requireOK(t, frame)
	var value settings.Record
	if err := json.Unmarshal(frame.Result, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestInvalidStoredSettingsReleaseStartupResources(t *testing.T) {
	runtime, cfg := runtimeFixture(t, 1)
	if err := runtime.db.DB.Table("app_settings").Where("key=?", settings.ExecutionKey).Update("value", `{"approval_policy":"never","approvals_reviewer":"auto_review"}`).Error; err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if failed, err := newTestRuntime(t.Context(), cfg, slog.New(slog.NewJSONHandler(io.Discard, nil))); err == nil {
		failed.Close()
		t.Fatal("非法持久化配置被默认权限替代")
	} else if !errors.Is(err, settings.ErrStorage) {
		t.Fatalf("启动错误丢失设置来源: %v", err)
	}
	// 只修复临时测试库，之后应能重新取得实例锁并正常启动。
	db, err := sqlite.Open(t.Context(), cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	err = db.DB.Table("app_settings").Where("key=?", settings.ExecutionKey).Update("value", `{"approval_policy":"on-request","approvals_reviewer":"user"}`).Error
	closeErr := db.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("临时设置修复失败: %v %v", err, closeErr)
	}
	restarted, err := newTestRuntime(t.Context(), cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSettingsStdioConflictAndRestart(t *testing.T) {
	runtime, cfg := runtimeFixture(t, 1)
	first, second := sharedPeer(t, runtime), sharedPeer(t, runtime)
	initial := readSettings(t, first)
	setApprovalPolicy(t, first, "always")
	saved := readSettings(t, second)
	if saved.Revision != initial.Revision+1 || saved.ID != initial.ID {
		t.Fatalf("其他连接未读到新设置: %+v", saved)
	}
	conflict := second.call(t, "settings.update", map[string]any{"key": "execution", "revision": initial.Revision,
		"value": map[string]any{"approval_policy": "never", "approvals_reviewer": "user"}})
	if conflict.Error == nil || conflict.Error.Code != 1003 {
		t.Fatal("过期设置覆盖了新设置")
	}
	for _, params := range []map[string]any{
		{"key": "unknown", "revision": saved.Revision, "value": map[string]any{}},
		{"key": "execution", "revision": 0, "value": map[string]any{}},
		{"key": "execution", "revision": saved.Revision, "value": nil},
		{"key": "execution", "revision": saved.Revision, "value": map[string]any{"approval_policy": "never", "approvals_reviewer": "auto_review"}},
	} {
		frame := second.call(t, "settings.update", params)
		if frame.Error == nil || frame.Error.Code != -32602 {
			t.Fatalf("非法配置未拒绝: %+v", frame)
		}
	}
	first.stop(t)
	second.stop(t)
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := newTestRuntime(t.Context(), cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := restarted.Close(); err != nil {
			t.Error(err)
		}
	}()
	value, err := restarted.Settings().Get(t.Context(), settings.ExecutionKey)
	if err != nil || value.Revision != saved.Revision || string(value.Value) != string(saved.Value) || value.ID != saved.ID {
		t.Fatalf("重启覆盖了用户配置: %+v %v", value, err)
	}
}

func TestSettingsUpdateIsolatesRunningApprovalAndNewRun(t *testing.T) {
	type projection struct {
		Source   string `json:"source"`
		Revision int64  `json:"settings_revision"`
		Policy   string `json:"approval_policy"`
		Question string `json:"-"`
	}
	projections := make(chan projection, 8)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Tools    []json.RawMessage `json:"tools"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if len(input.Tools) > 0 {
			var value projection
			system := input.Messages[0].Content
			if json.Unmarshal([]byte(system[strings.LastIndex(system, "\n")+1:]), &value) != nil {
				t.Error("主请求缺少执行设置投影")
			}
			for _, message := range input.Messages {
				if message.Role == "user" && (message.Content == "旧任务" || message.Content == "新任务") {
					value.Question = message.Content
				}
			}
			select {
			case projections <- value:
			default:
				t.Error("模型请求超出预期轮次")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		count := 0
		for _, m := range input.Messages {
			if m.Role == "tool" {
				count++
			}
		}
		if len(input.Tools) == 0 || count >= 2 {
			_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"完成"},"finish_reason":"stop"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"call-`+strconv.Itoa(count+1)+`","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"a.txt\"}"}}]},"finish_reason":"tool_calls"}]}`)
	}))
	t.Cleanup(upstream.Close)
	runtime, _ := runtimeFixture(t, 2)
	p := sharedPeer(t, runtime)
	sub, err := runtime.Subscribe(events.Filter{Types: []string{"approval.requested", "run.completed", "run.failed"}})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	mid, wid := setupConversationModel(t, p, upstream.URL), setupToolWorkspace(t, p)
	setApprovalPolicy(t, p, "always")
	oldSettings := readSettings(t, p)
	send := func(text string) string {
		return resultString(t, p.call(t, "conversation.send", map[string]any{"model_id": mid, "workspace_id": wid, "stream": false, "content": messageBlocks(text)}), "run_id")
	}
	oldRun := send("旧任务")
	firstApproval := nextBusiness(t, sub, "approval.requested", oldRun).Data.(agent.Approval)
	setApprovalPolicy(t, p, "on-request")
	newSettings := readSettings(t, p)
	newRun := send("新任务")
	nextBusiness(t, sub, "run.completed", newRun)
	newState, err := runtime.Runs().Get(t.Context(), newRun)
	if err != nil || newState.SettingsSnapshot == nil || newState.SettingsSnapshot.Revision != newSettings.Revision || newState.SettingsSnapshot.ApprovalPolicy != "on-request" {
		t.Fatalf("新执行没有读取新设置: %+v %v", newState, err)
	}
	rows, err := runtime.Approvals().List(t.Context(), agent.ApprovalFilter{RunID: newRun})
	if err != nil || len(rows) != 0 {
		t.Fatalf("新执行仍使用旧策略: %+v %v", rows, err)
	}
	oldState, err := runtime.Runs().Get(t.Context(), oldRun)
	if err != nil || oldState.Status != agent.AwaitingApproval || oldState.SettingsSnapshot == nil || oldState.SettingsSnapshot.Revision != oldSettings.Revision {
		t.Fatalf("更新干扰了待审批任务: %+v %v", oldState, err)
	}
	requireOK(t, p.call(t, "approval.decide", map[string]any{"approval_id": firstApproval.ID, "decision": "approved"}))
	secondApproval := nextBusiness(t, sub, "approval.requested", oldRun).Data.(agent.Approval)
	if secondApproval.ID == firstApproval.ID {
		t.Fatal("同一次审批被当作下一步授权")
	}
	requireOK(t, p.call(t, "approval.decide", map[string]any{"approval_id": secondApproval.ID, "decision": "approved"}))
	nextBusiness(t, sub, "run.completed", oldRun)
	for range 6 { // 两个任务各经过两轮工具调用及一轮最终回答。
		var value projection
		select {
		case value = <-projections:
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		}
		want := oldSettings
		if value.Question == "新任务" {
			want = newSettings
		} else if value.Question != "旧任务" {
			t.Fatalf("无法确定请求所属任务: %+v", value)
		}
		var execution settings.Execution
		if err := json.Unmarshal(want.Value, &execution); err != nil {
			t.Fatal(err)
		}
		if value.Source != "server_execution_settings" || value.Revision != want.Revision || value.Policy != string(execution.ApprovalPolicy) {
			t.Fatalf("模型请求混用了其他执行的设置: %+v want=%+v", value, want)
		}
	}
	steps, err := runtime.Runs().Steps(t.Context(), oldRun, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range steps {
		if step.Type != "tool_call" {
			continue
		}
		var input struct {
			Revision int64  `json:"settings_revision"`
			Policy   string `json:"approval_policy"`
		}
		if err := json.Unmarshal(step.Input, &input); err != nil {
			t.Fatal(err)
		}
		if input.Revision != oldSettings.Revision || input.Policy != "always" {
			t.Fatalf("运行中任务混用了新设置: %s", step.Input)
		}
	}
	p.stop(t)
}
