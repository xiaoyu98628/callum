package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/model"
)

func TestSkillReferencesThroughSendResendAndRestart(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var requests []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		mu.Lock()
		requests = append(requests, string(data))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"完成"},"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	p := startApp(t, cfg, io.Discard)
	mid := setupConversationModel(t, p, upstream.URL)
	dir := filepath.Join(cfg.DataDir, "skills", "review")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte("---\nname: Review\ndescription: explicit review\n---\n核对任务。"), 0600); err != nil {
		t.Fatal(err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	content := []model.Block{{Type: "skill", Name: "Review", Path: path}, {Type: "text", Text: "核对正文"}}
	for _, invalid := range []model.Block{
		{Type: "skill", Name: "Review", Path: filepath.Join(t.TempDir(), "SKILL.md")},
		{Type: "skill", Name: "wrong-name", Path: path},
	} {
		bad := p.call(t, "conversation.send", map[string]any{"model_id": mid, "content": []model.Block{invalid, content[1]}})
		if bad.Error == nil || bad.Error.Code != -32602 {
			t.Fatalf("非法技能被受理: %+v", bad)
		}
	}
	sent := p.call(t, "conversation.send", map[string]any{"model_id": mid, "stream": false, "content": content})
	cid, uid := resultString(t, sent, "conversation_id"), resultString(t, sent, "user_message_id")
	p.event(t, "run.completed")
	rows := readMessages(t, p, cid)
	if len(rows) != 2 || len(rows[0].Content) != 2 || rows[0].Content[0] != content[0] || rows[0].Content[1] != content[1] {
		t.Fatalf("技能未独立保存: %+v", rows)
	}
	content[1].Text = "修改后的正文"
	resend := p.call(t, "conversation.resend", map[string]any{"conversation_id": cid, "message_id": uid, "stream": false, "content": content})
	requireOK(t, resend)
	p.event(t, "run.completed")
	p.stop(t)
	p = startApp(t, cfg, io.Discard)
	rows = readMessages(t, p, cid)
	if rows[0].Content[0] != content[0] || rows[0].Content[1] != content[1] {
		t.Fatalf("重启后引用丢失: %+v", rows)
	}
	mu.Lock()
	joined := strings.Join(requests, "\n")
	mu.Unlock()
	if !strings.Contains(joined, "$Review 核对正文") || !strings.Contains(joined, "$Review 修改后的正文") || strings.Contains(joined, "请使用技能") {
		t.Fatalf("模型引用格式错误: %s", joined)
	}
	var seen bool
	for _, request := range requests {
		var parsed struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if json.Unmarshal([]byte(request), &parsed) != nil {
			continue
		}
		for _, message := range parsed.Messages {
			if strings.Contains(message.Content, `"skills":[`) && strings.Contains(message.Content, path) {
				seen = true
			}
		}
	}
	if !seen {
		t.Fatal("模型缺少所选技能的定位元数据")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	bad := p.call(t, "conversation.resend", map[string]any{"conversation_id": cid, "message_id": uid, "content": content})
	if bad.Error == nil || bad.Error.Code != -32602 {
		t.Fatalf("删除后的技能仍可重发: %+v", bad)
	}
}
