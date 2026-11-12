package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/tokens"
	"github.com/xiaoyu98628/callum/internal/settings"
)

func correctionCall(id, name, args string, stream bool, index int) map[string]any {
	call := map[string]any{"id": id, "type": "function", "function": map[string]string{"name": name, "arguments": args}}
	if stream {
		call["index"] = index
	}
	return call
}
func correctionJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
func correctionSteps(t *testing.T, p *appPeer, runID string) []agent.StepSummary {
	t.Helper()
	frame := p.call(t, "step.list", map[string]any{"run_id": runID, "limit": 200})
	requireOK(t, frame)
	var steps []agent.StepSummary
	if err := json.Unmarshal(frame.Result, &steps); err != nil {
		t.Fatal(err)
	}
	return steps
}
func correctionData(t *testing.T, p *appPeer, stepID, field string) string {
	t.Helper()
	frame := p.call(t, "step.data", map[string]any{"step_id": stepID, "field": field})
	requireOK(t, frame)
	var page struct {
		Fragment string
		Done     bool
	}
	if err := json.Unmarshal(frame.Result, &page); err != nil || !page.Done {
		t.Fatalf("步骤审计读取失败: %s %v", frame.Result, err)
	}
	return page.Fragment
}

func TestMalformedArgumentsCorrectionKeepsAuditAndApproval(t *testing.T) {
	t.Parallel()
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			marker, never := filepath.Join(outside, "corrected.txt"), filepath.Join(outside, "never.txt")
			if err := os.WriteFile(filepath.Join(root, "read.txt"), []byte("already-read"), 0600); err != nil {
				t.Fatal(err)
			}
			var mainCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var input struct {
					Stream   bool
					Tools    []json.RawMessage
					Messages []struct {
						Role, Content string
						Calls         []struct{ ID string } `json:"tool_calls"`
					}
				}
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if len(input.Tools) == 0 {
					writeFileToolResponse(t, w, input.Stream, map[string]any{"content": "title"}, "stop")
					return
				}
				number := mainCalls.Add(1)
				readCount, badCount, feedback := 0, 0, false
				for _, m := range input.Messages {
					if strings.Contains(m.Content, "服务端工具参数校验反馈") {
						feedback = true
					}
					for _, call := range m.Calls {
						if call.ID == "read" {
							readCount++
						}
						if strings.HasPrefix(call.ID, "bad-") {
							badCount++
						}
					}
				}
				if number >= 3 && (readCount != 1 || badCount != 0) {
					t.Errorf("纠正重放了先前操作或将坏批次带入上下文: read=%d bad=%d", readCount, badCount)
				}
				if number == 3 && !feedback {
					t.Error("未发送服务端纠正信息")
				}
				if number > 3 && feedback {
					t.Error("纠正信息污染了后续历史")
				}
				switch number {
				case 1:
					args := correctionJSON(t, map[string]any{"path": "read.txt"})
					writeFileToolResponse(t, w, input.Stream, map[string]any{"tool_calls": []any{correctionCall("read", "read_file", args, input.Stream, 0)}}, "tool_calls")
				case 2:
					patch := correctionJSON(t, map[string]any{"patch": "*** Begin Patch\n*** Add File: " + filepath.ToSlash(never) + "\n+must not execute\n*** End Patch\n", "base_versions": []any{}})
					writeFileToolResponse(t, w, input.Stream, map[string]any{"content": "bad batch", "tool_calls": []any{correctionCall("bad-valid", "apply_patch", patch, input.Stream, 0), correctionCall("bad-json", "read_file", `{"["string""]</｜｜DSML｜｜: `, input.Stream, 1)}}, "tool_calls")
				case 3:
					patch := correctionJSON(t, map[string]any{"patch": "*** Begin Patch\n*** Add File: " + filepath.ToSlash(marker) + "\n+corrected only\n*** End Patch\n", "base_versions": []any{}})
					writeFileToolResponse(t, w, input.Stream, map[string]any{"tool_calls": []any{correctionCall("corrected", "apply_patch", patch, input.Stream, 0)}}, "tool_calls")
				default:
					writeFileToolResponse(t, w, input.Stream, map[string]any{"content": "done"}, "stop")
				}
			}))
			t.Cleanup(server.Close)
			cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
			if err != nil {
				t.Fatal(err)
			}
			p := startApp(t, cfg, io.Discard)
			mid := setupConversationModel(t, p, server.URL)
			wid := resultString(t, p.call(t, "workspace.create", map[string]any{"name": "project", "root_path": root}), "id")
			accepted := p.call(t, "conversation.send", map[string]any{"model_id": mid, "workspace_id": wid, "stream": stream, "content": messageBlocks("recover")})
			requireOK(t, accepted)
			rid, cid := resultString(t, accepted, "run_id"), resultString(t, accepted, "conversation_id")
			event := p.event(t, "approval.requested")
			var pending struct {
				ApprovalID string         `json:"approval_id"`
				Approval   agent.Approval `json:"approval"`
			}
			if err := json.Unmarshal(event.Params, &pending); err != nil {
				t.Fatal(err)
			}
			if mainCalls.Load() != 3 || pending.Approval.RequestHash == "" || !bytes.Contains(pending.Approval.Request, []byte("corrected.txt")) {
				t.Fatalf("纠正后未重新生成审批: calls=%d %s", mainCalls.Load(), event.Params)
			}
			for _, file := range []string{never, marker} {
				if _, err := os.Stat(file); !os.IsNotExist(err) {
					t.Fatalf("审批前或坏批次已经执行: %s %v", file, err)
				}
			}
			requireOK(t, p.call(t, "approval.decide", map[string]any{"approval_id": pending.ApprovalID, "decision": "approved"}))
			p.event(t, "run.completed")
			data, err := os.ReadFile(marker)
			if err != nil || string(data) != "corrected only\n" {
				t.Fatalf("纠正后的操作没有完成: %q %v", data, err)
			}
			if _, err := os.Stat(never); !os.IsNotExist(err) {
				t.Fatal("执行了坏批次中的有效调用")
			}
			toolCount, failedModel, corrections := 0, 0, 0
			for _, step := range correctionSteps(t, p, rid) {
				if step.Type == "tool_call" {
					toolCount++
				}
				if step.Type == "model_call" {
					input := correctionData(t, p, step.ID, "input")
					if strings.Contains(input, "tool_arguments_correction") {
						corrections++
					}
					if step.Status == agent.Failed {
						failedModel++
						output := correctionData(t, p, step.ID, "output")
						if !strings.Contains(output, "DSML") || strings.Contains(output, "context_message") || step.UsageBytes == 0 {
							t.Fatalf("坏响应审计或 usage 丢失: %s", output)
						}
					}
				}
			}
			if toolCount != 2 || failedModel != 1 || corrections != 1 {
				t.Fatalf("步骤未保持操作次数和纠正来源: tools=%d failed=%d corrections=%d", toolCount, failedModel, corrections)
			}
			// 后续会话恢复只保留已执行事实，不恢复坏批次或临时纠正指令。
			requireOK(t, p.call(t, "conversation.send", map[string]any{"conversation_id": cid, "model_id": mid, "stream": stream, "content": messageBlocks("继续")}))
			p.event(t, "run.completed")
		})
	}
}

func TestRepeatedMalformedArgumentsStopWithoutExecuting(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"json", "schema"} {
		t.Run(mode, func(t *testing.T) {
			var count atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var input struct {
					Stream   bool
					Tools    []json.RawMessage
					Messages []struct{ Role, Content string }
				}
				if json.NewDecoder(r.Body).Decode(&input) != nil {
					w.WriteHeader(400)
					return
				}
				if len(input.Tools) == 0 {
					writeFileToolResponse(t, w, input.Stream, map[string]any{"content": "title"}, "stop")
					return
				}
				n := count.Add(1)
				args := `{"DSML`
				if mode == "schema" {
					args = `{"command":"[\"git\",\"status\"]"}`
				}
				if n == 2 && mode == "schema" && !strings.Contains(input.Messages[len(input.Messages)-1].Content, "字符串数组") {
					t.Error("数组类型错误没有字段级反馈")
				}
				writeFileToolResponse(t, w, input.Stream, map[string]any{"tool_calls": []any{correctionCall(fmt.Sprint(n), "run_command", args, input.Stream, 0)}}, "tool_calls")
			}))
			t.Cleanup(server.Close)
			cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
			if err != nil {
				t.Fatal(err)
			}
			p := startApp(t, cfg, io.Discard)
			mid := setupConversationModel(t, p, server.URL)
			accepted := p.call(t, "conversation.send", map[string]any{"model_id": mid, "stream": true, "content": messageBlocks("repeat")})
			requireOK(t, accepted)
			terminal := p.event(t, "run.failed")
			if !bytes.Contains(terminal.Params, []byte("没有执行进展")) || count.Load() != 2 {
				t.Fatalf("重复坏参数没有明确终止: count=%d %s", count.Load(), terminal.Params)
			}
			rid := resultString(t, accepted, "run_id")
			for _, step := range correctionSteps(t, p, rid) {
				if step.Type == "tool_call" && step.Status != agent.Failed {
					t.Fatalf("执行了坏参数工具: %+v", step)
				}
			}
		})
	}
}

func TestArgumentCorrectionCancellationAndConversationIsolation(t *testing.T) {
	t.Parallel()
	waiting, cancelled := make(chan struct{}), make(chan struct{})
	var badCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Stream   bool
			Tools    []json.RawMessage
			Messages []struct{ Role, Content string }
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			w.WriteHeader(400)
			return
		}
		if len(input.Tools) == 0 {
			writeFileToolResponse(t, w, input.Stream, map[string]any{"content": "title"}, "stop")
			return
		}
		bad, feedback := false, false
		for _, m := range input.Messages {
			if m.Role == "user" && m.Content == "bad-isolated" {
				bad = true
			}
			if strings.Contains(m.Content, "服务端工具参数校验反馈") {
				feedback = true
			}
		}
		if !bad {
			if feedback {
				t.Error("参数纠正信息串入其他会话")
			}
			writeFileToolResponse(t, w, input.Stream, map[string]any{"content": "healthy"}, "stop")
			return
		}
		n := badCalls.Add(1)
		if n == 1 {
			writeFileToolResponse(t, w, input.Stream, map[string]any{"tool_calls": []any{correctionCall("bad", "read_file", "{", input.Stream, 0)}}, "tool_calls")
			return
		}
		if n != 2 || !feedback {
			t.Error("纠正请求状态错误")
			w.WriteHeader(500)
			return
		}
		close(waiting)
		<-r.Context().Done()
		close(cancelled)
	}))
	t.Cleanup(server.Close)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConcurrentRuns = 2
	p := startApp(t, cfg, io.Discard)
	mid := setupConversationModel(t, p, server.URL)
	bad := p.call(t, "conversation.send", map[string]any{"model_id": mid, "stream": true, "content": messageBlocks("bad-isolated")})
	requireOK(t, bad)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	select {
	case <-waiting:
	case <-ctx.Done():
		t.Fatal("没有进入纠正请求:", ctx.Err())
	}
	healthy := p.call(t, "conversation.send", map[string]any{"model_id": mid, "stream": false, "content": messageBlocks("healthy-isolated")})
	requireOK(t, healthy)
	p.event(t, "run.completed")
	for _, step := range correctionSteps(t, p, resultString(t, healthy, "run_id")) {
		if step.Type == "model_call" && strings.Contains(correctionData(t, p, step.ID, "input"), "tool_arguments_correction") {
			t.Fatal("纠正审计串入其他执行")
		}
	}
	requireOK(t, p.call(t, "run.cancel", map[string]any{"run_id": resultString(t, bad, "run_id")}))
	p.event(t, "run.cancelled")
	select {
	case <-cancelled:
	case <-ctx.Done():
		t.Fatal("取消未传递给纠正请求:", ctx.Err())
	}
	if badCalls.Load() != 2 {
		t.Fatal("取消后仍启动纠正")
	}
}

func TestArgumentCorrectionFeedbackCountsTowardBudget(t *testing.T) {
	t.Parallel()
	var count atomic.Int32
	var baseline atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Stream   bool
			Tools    []struct{ Function model.ToolDefinition }
			Messages []model.Message
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			w.WriteHeader(400)
			return
		}
		if len(input.Tools) == 0 {
			writeFileToolResponse(t, w, input.Stream, map[string]any{"content": "title"}, "stop")
			return
		}
		n := count.Add(1)
		if n == 1 {
			var defs []model.ToolDefinition
			for _, item := range input.Tools {
				defs = append(defs, item.Function)
			}
			baseline.Store(tokens.Estimate(input.Messages, defs, 0))
			writeFileToolResponse(t, w, input.Stream, map[string]any{"content": "baseline"}, "stop")
			return
		}
		writeFileToolResponse(t, w, input.Stream, map[string]any{"tool_calls": []any{correctionCall("bad", "read_file", "{", input.Stream, 0)}}, "tool_calls")
	}))
	t.Cleanup(server.Close)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	p := startApp(t, cfg, io.Discard)
	mid := setupConversationModel(t, p, server.URL)
	requireOK(t, p.call(t, "conversation.send", map[string]any{"model_id": mid, "stream": false, "content": messageBlocks("same question")}))
	p.event(t, "run.completed")
	compression := settings.DefaultCompaction()
	compression.Enabled = true
	compression.TokenBudget = baseline.Load() + 1
	compactionSetting(t, p, compression)
	requireOK(t, p.call(t, "conversation.send", map[string]any{"model_id": mid, "stream": false, "content": messageBlocks("same question")}))
	terminal := p.event(t, "run.failed")
	if count.Load() != 2 || !bytes.Contains(terminal.Params, []byte("token_budget_exceeded")) {
		t.Fatalf("纠正信息绕过预算或第一轮未进入模型: calls=%d %s", count.Load(), terminal.Params)
	}
}
