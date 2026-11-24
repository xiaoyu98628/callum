package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/provider"
)

func TestContinueRejectsIncompatibleModelBeforeChangingRun(t *testing.T) {
	rt, _ := runtimeFixture(t, 1)
	ctx := t.Context()
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil || !bytes.Contains(body, []byte("data:image/png;base64,")) {
			t.Errorf("继续请求缺少原图片: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"已继续"},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(upstream.Close)
	pid, err := rt.Providers().CreateProvider(ctx, provider.ProviderInput{Name: "厂商", Protocol: "chatcompletions", BaseURL: upstream.URL + "/v1"})
	if err != nil {
		t.Fatal(err)
	}
	addModel := func(name string, types ...string) string {
		t.Helper()
		id, err := rt.Providers().CreateModel(ctx, provider.ModelInput{ProviderID: pid, DisplayName: name, RemoteModelID: name,
			Enabled: true, ModelType: types, ContextWindow: new(int64(1000000))})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	vision, text := addModel("vision", "image"), addModel("text", "text")
	var original bytes.Buffer
	if err := png.Encode(&original, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "resume.png")
	if err := os.WriteFile(path, original.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	svc := rt.Conversations()
	var initial conversation.Execution
	err = svc.Send(ctx, conversation.Send{ModelID: vision, Stream: new(false), Content: []model.Block{{Type: "image", Path: path}}}, func(ex conversation.Execution) error {
		initial = ex
		if !rt.Cancel(ex.Run.ID) {
			t.Fatal("受理阶段无法取消任务")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	id, run := initial.Conversation.ID, initial.Run.ID
	rt.manager.Wait(run)
	before, err := svc.History(ctx, id, "", 25)
	if err != nil || len(before.Runs) != 1 || before.Runs[0].Status != agent.Cancelled {
		t.Fatalf("未准备好已停止的任务: %+v %v", before, err)
	}
	steps, err := rt.Runs().Steps(ctx, run, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	p := sharedPeer(t, rt)
	params := map[string]any{"conversation_id": id, "run_id": run, "model_id": text}
	if rejected := p.call(t, "conversation.continue", params); rejected.Error == nil || rejected.Error.Code != 1005 {
		t.Fatalf("不兼容的恢复未返回既有错误: %+v", rejected)
	}
	after, err := svc.History(ctx, id, "", 25)
	if err != nil || !reflect.DeepEqual(before, after) || calls.Load() != 0 {
		t.Fatalf("拒绝恢复后原执行状态被修改或调用了模型: %v", err)
	}
	afterSteps, err := rt.Runs().Steps(ctx, run, 0, 20)
	if err != nil || !reflect.DeepEqual(steps, afterSteps) {
		t.Fatalf("拒绝恢复后改写了执行步骤: %v", err)
	}
	saved, err := svc.Get(ctx, id)
	if err != nil || saved.Model.ID != vision {
		t.Fatalf("拒绝恢复后改写了会话模型: %+v %v", saved.Model, err)
	}
	params["model_id"] = vision
	if got := resultString(t, p.call(t, "conversation.continue", params), "run_id"); got != run {
		t.Fatalf("继续使用了不同执行: %q", got)
	}
	p.event(t, "run.completed")
	if calls.Load() != 1 {
		t.Fatalf("模型调用次数: %d", calls.Load())
	}
	p.stop(t)
}

func TestAttachmentHistoryResendAndRecovery(t *testing.T) {
	var original bytes.Buffer
	if err := png.Encode(&original, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	expected := "data:image/png;base64," + base64.StdEncoding.EncodeToString(original.Bytes())
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if !bytes.Contains(data, []byte(expected)) {
			t.Error("模型未收到原图的完整字节")
		}
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"图片已收到"},"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConcurrentRuns = 1
	p := startApp(t, cfg, io.Discard)
	providerID := resultString(t, p.call(t, "provider.create", map[string]any{
		"name": "附件测试", "protocol": "chatcompletions", "base_url": upstream.URL + "/v1",
	}), "id")
	modelID := resultString(t, p.call(t, "model.create", map[string]any{
		"provider_id": providerID, "display_name": "附件模型", "remote_model_id": "vision",
		"enabled": true, "context_window": 1000000, "model_type": []string{"image"},
	}), "id")
	path := filepath.Join(t.TempDir(), "original.png")
	if err := os.WriteFile(path, original.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	first := p.call(t, "conversation.send", map[string]any{
		"model_id": modelID, "stream": false, "content": []map[string]string{{"type": "image", "path": path}},
	})
	conv := resultString(t, first, "conversation_id")
	run := resultString(t, first, "run_id")
	user := resultString(t, first, "user_message_id")
	p.event(t, "run.completed")
	history := p.call(t, "conversation.history", map[string]any{"conversation_id": conv})
	var page conversation.HistoryPage
	requireOK(t, history)
	if err := json.Unmarshal(history.Result, &page); err != nil {
		t.Fatal(err)
	}
	block := page.Messages[0].Content[0]
	if block.Path != "" || block.AttachmentRef == "" {
		t.Fatalf("持久化图片未使用相对引用: %+v", block)
	}
	attachmentDir := filepath.Join(cfg.DataDir, "attachments")
	owned := filepath.Join(attachmentDir, filepath.FromSlash(block.AttachmentRef))
	saved, err := os.ReadFile(owned)
	if err != nil || !bytes.Equal(saved, original.Bytes()) {
		t.Fatalf("副本不一致: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	p.stop(t)
	// 模拟已提交后尚未删除的标记，以及提交前中断留下的附件。
	pending := filepath.Join(attachmentDir, ".pending")
	orphanID := uuid.Must(uuid.NewV7()).String()
	for _, id := range []string{run, orphanID} {
		if err := os.WriteFile(filepath.Join(pending, conv+"_"+id), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	orphan := filepath.Join(attachmentDir, conv, orphanID)
	if err := os.Mkdir(orphan, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(orphan, filepath.Base(owned)), original.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	p = startApp(t, cfg, io.Discard)
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("未回收未受理的目录: %v", err)
	}
	entries, err := os.ReadDir(pending)
	if err != nil || len(entries) != 0 {
		t.Fatalf("恢复标记未清理: %v %v", entries, err)
	}
	preview := p.call(t, "conversation.attachment_preview", map[string]any{
		"conversation_id": conv, "message_id": user, "index": 0,
	})
	requireOK(t, preview)
	var view struct {
		MIME string `json:"mime"`
		Data []byte `json:"data"`
	}
	if err := json.Unmarshal(preview.Result, &view); err != nil || view.MIME != "image/png" || !bytes.Equal(view.Data, original.Bytes()) {
		t.Fatalf("重启预览不一致: %v", err)
	}
	read := func(expected string) wireFrame {
		return p.call(t, "conversation.attachment_read", map[string]any{
			"conversation_id": conv, "message_id": user, "expected_run_id": expected, "index": 0, "offset": 0,
		})
	}
	firstRead := read(run)
	requireOK(t, firstRead)
	var chunk conversation.AttachmentChunk
	digest := sha256.Sum256(original.Bytes())
	if err := json.Unmarshal(firstRead.Result, &chunk); err != nil || chunk.RunID != run ||
		chunk.SHA256 != hex.EncodeToString(digest[:]) || !bytes.Equal(chunk.Data, original.Bytes()) {
		t.Fatalf("读取结果未绑定附件与消息版本: %+v %v", chunk, err)
	}
	resend := p.call(t, "conversation.resend", map[string]any{
		"conversation_id": conv, "message_id": user, "model_id": modelID, "stream": false,
		"expected_run_id": run, "retained_attachments": []int{0}, "content": []any{},
	})
	requireOK(t, resend)
	if stale := read(run); stale.Error == nil || stale.Error.Code != 1003 {
		t.Fatalf("旧消息版本仍可另存: %+v", stale)
	}
	requireOK(t, read(resultString(t, resend, "run_id")))
	p.event(t, "run.completed")
	if calls.Load() != 2 {
		t.Fatalf("期待原图和重发两次调用，得到 %d", calls.Load())
	}
	p.stop(t)
}
