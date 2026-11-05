package app

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/config"
)

func TestConversationDirectoryApprovalThroughStdio(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "child", "note.txt")
	if err := os.MkdirAll(filepath.Dir(child), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(child, []byte("directory-permission-content"), 0600); err != nil {
		t.Fatal(err)
	}
	outside := root + "-sibling"
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(outside); err != nil {
			t.Error(err)
		}
	})
	other := filepath.Join(outside, "outside.txt")
	if err := os.WriteFile(other, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	created := filepath.Join(root, "new.txt")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Stream   bool              `json:"stream"`
			Tools    []json.RawMessage `json:"tools"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
				CallID  string `json:"tool_call_id"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if len(req.Tools) == 0 {
			writeFileToolResponse(t, w, req.Stream, map[string]any{"content": "目录授权"}, "stop")
			return
		}
		text := ""
		for _, m := range req.Messages {
			if m.Role == "user" {
				text = m.Content
			}
		}
		last := req.Messages[len(req.Messages)-1]
		name, args, id := "read_file", map[string]any{"path": child}, "read-child"
		if last.Role == "tool" && !(text == "grant" && last.CallID == "list-root") {
			writeFileToolResponse(t, w, req.Stream, map[string]any{"content": "完成"}, "stop")
			return
		}
		switch text {
		case "grant":
			if last.Role != "tool" {
				name, args, id = "list_directory", map[string]any{"path": root}, "list-root"
			}
		case "sibling":
			args["path"] = other
		case "symlink":
			args["path"] = filepath.Join(link, "outside.txt")
		case "write", "write-child":
			path := created
			if text == "write-child" {
				path = filepath.Join(root, "nested", "created.txt")
			}
			name, args = "apply_patch", map[string]any{"patch": "*** Begin Patch\n*** Add File: " + path + "\n+created through directory approval\n*** End Patch", "base_versions": []any{}}
		case "command":
			name, args = "run_command", map[string]any{"command": []string{"echo", "no-execution"}, "cwd": root}
		case "destructive":
			name, args = "apply_patch", map[string]any{"patch": "*** Begin Patch\n*** Update File: " + child + "\n@@\n-directory-permission-content\n+replacement\n*** End Patch", "base_versions": []any{map[string]string{"path": child, "sha256": fmt.Sprintf("%x", sha256.Sum256([]byte("directory-permission-content")))}}}
		}
		raw, err := json.Marshal(args)
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		call := map[string]any{"id": id, "type": "function", "function": map[string]any{"name": name, "arguments": string(raw)}}
		if req.Stream {
			call["index"] = 0
		}
		writeFileToolResponse(t, w, req.Stream, map[string]any{"tool_calls": []any{call}}, "tool_calls")
	}))
	defer upstream.Close()
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	p := startApp(t, cfg, io.Discard)
	mid := setupConversationModel(t, p, upstream.URL)
	var cid string
	start := func(text string, fresh bool) string {
		params := map[string]any{"stream": true, "content": messageBlocks(text)}
		if fresh || cid == "" {
			params["model_id"] = mid
		} else {
			params["conversation_id"] = cid
		}
		frame := p.call(t, "conversation.send", params)
		if cid == "" {
			cid = resultString(t, frame, "conversation_id")
		}
		return resultString(t, frame, "run_id")
	}
	decide := func(scope, decision string, wantOption bool) agent.Approval {
		var event struct {
			Approval agent.Approval `json:"approval"`
		}
		if err := json.Unmarshal(p.event(t, "approval.requested").Params, &event); err != nil {
			t.Fatal(err)
		}
		if (event.Approval.DirectoryScope != nil) != wantOption {
			t.Fatalf("目录选项不符: %+v", event.Approval)
		}
		for _, invalid := range []any{"", "global", nil, 1, map[string]any{"root_path": root}} {
			frame := p.call(t, "approval.decide", map[string]any{"approval_id": event.Approval.ID, "decision": decision, "scope": invalid})
			if frame.Error == nil || frame.Error.Code != -32602 {
				t.Fatalf("接受非法授权范围: %+v", frame)
			}
		}
		requireOK(t, p.call(t, "approval.decide", map[string]any{"approval_id": event.Approval.ID, "decision": decision, "scope": scope}))
		return event.Approval
	}
	assertApprovals := func(runID string, want int) {
		frame := p.call(t, "approval.list", map[string]any{"run_id": runID})
		requireOK(t, frame)
		var rows []agent.Approval
		if err := json.Unmarshal(frame.Result, &rows); err != nil {
			t.Fatal(err)
		}
		if len(rows) != want {
			t.Fatalf("审批次数不符: want=%d actual=%s", want, frame.Result)
		}
	}
	first := start("grant", false)
	a := decide(agent.ApprovalConversationDirectory, agent.ApprovalApproved, true)
	if a.DirectoryScope.RootPath != root || a.DirectoryScope.Access != "read" {
		t.Fatal("目录范围或影响类别错误")
	}
	p.event(t, "run.completed")
	assertApprovals(first, 1) // 同轮的子目录读取继承授权。
	list := p.call(t, "approval.directory.list", map[string]any{"conversation_id": cid})
	requireOK(t, list)
	var grants []agent.DirectoryGrant
	if err := json.Unmarshal(list.Result, &grants); err != nil || len(grants) != 1 {
		t.Fatalf("未保存目录授权: %s %v", list.Result, err)
	}
	repeat := p.call(t, "approval.decide", map[string]any{"approval_id": a.ID, "decision": "approved", "scope": agent.ApprovalOnce})
	if repeat.Error == nil || repeat.Error.Code != 1003 {
		t.Fatal("已决定的授权范围被更改")
	}
	p.stop(t)
	p = startApp(t, cfg, io.Discard)
	read := start("read", false)
	p.event(t, "run.completed")
	assertApprovals(read, 0)
	for _, text := range []string{"sibling", "symlink", "write", "command"} {
		run := start(text, false)
		decide(agent.ApprovalOnce, agent.ApprovalRejected, text != "command")
		p.event(t, "run.completed")
		assertApprovals(run, 1)
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Fatal("读取授权扩大成了写权限")
	}
	otherRun := start("read", true)
	decide(agent.ApprovalOnce, agent.ApprovalRejected, true)
	p.event(t, "run.completed")
	assertApprovals(otherRun, 1)
	writeRun := start("write", false)
	decide(agent.ApprovalConversationDirectory, agent.ApprovalApproved, true)
	p.event(t, "run.completed")
	assertApprovals(writeRun, 1)
	if _, err := os.Stat(created); err != nil {
		t.Fatalf("明确的目录写授权未生效: %v", err)
	}
	childWrite := start("write-child", false)
	p.event(t, "run.completed")
	assertApprovals(childWrite, 0)
	if _, err := os.Stat(filepath.Join(root, "nested", "created.txt")); err != nil {
		t.Fatalf("目录写授权未覆盖子目录: %v", err)
	}
	destructive := start("destructive", false)
	decide(agent.ApprovalOnce, agent.ApprovalRejected, false)
	p.event(t, "run.completed")
	assertApprovals(destructive, 1)
	if actual, err := os.ReadFile(child); err != nil || string(actual) != "directory-permission-content" {
		t.Fatalf("目录读写授权放行了大范围覆盖: %q %v", actual, err)
	}
	for _, mode := range []string{"always", "never"} {
		setApprovalPolicy(t, p, mode)
		run := start("read", false)
		if mode == "always" {
			decide(agent.ApprovalOnce, agent.ApprovalRejected, false)
		}
		p.event(t, "run.completed")
		want := 0
		if mode == "always" {
			want = 1
		}
		assertApprovals(run, want)
	}
	setApprovalPolicy(t, p, "on-request")
	frame := p.call(t, "approval.directory.list", map[string]any{"conversation_id": cid})
	requireOK(t, frame)
	if err := json.Unmarshal(frame.Result, &grants); err != nil || len(grants) != 2 {
		t.Fatalf("写授权缺失: %s %v", frame.Result, err)
	}
	for _, g := range grants {
		requireOK(t, p.call(t, "approval.directory.revoke", map[string]any{"conversation_id": cid, "id": g.ID}))
	}
	run := start("read", false)
	decide(agent.ApprovalOnce, agent.ApprovalRejected, true)
	p.event(t, "run.completed")
	assertApprovals(run, 1)
	p.stop(t)
}
