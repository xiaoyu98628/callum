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
	"github.com/xiaoyu98628/callum/internal/agent/events"
	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/conversation/history"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/tool"
)

func TestAgentToolsAndApprovalThroughStdio(t *testing.T) {
	const preamble = "我会先读取文件，确认内容后再回答。"
	for _, mode := range []string{"automatic", "never", "approve", "reject", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			automatic := mode == "automatic" || mode == "never"
			var toolResults atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var input struct {
					Stream   bool              `json:"stream"`
					Tools    []json.RawMessage `json:"tools"`
					Messages []struct {
						Role       string            `json:"role"`
						Content    string            `json:"content"`
						ToolCallID string            `json:"tool_call_id"`
						Calls      []json.RawMessage `json:"tool_calls"`
					} `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if len(input.Tools) != 6 {
					t.Errorf("未提供六个工具定义: %d", len(input.Tools))
				}
				if len(input.Messages) < 2 || input.Messages[0].Role != "system" || !strings.Contains(input.Messages[0].Content, "1～2句话") || !strings.Contains(input.Messages[0].Content, "正文 content") {
					t.Error("主对话未发送操作前说明的固定提示")
				}
				for _, message := range input.Messages[1:] {
					if message.Role == "system" {
						t.Error("多轮工具上下文重复累计了固定系统提示")
					}
				}
				last := input.Messages[len(input.Messages)-1]
				if last.Role == "tool" {
					toolResults.Add(1)
					if last.ToolCallID != "call-read" || len(input.Messages) < 3 || len(input.Messages[len(input.Messages)-2].Calls) != 1 {
						t.Error("工具调用与结果未配对")
					}
					if automatic || mode == "approve" {
						if !bytes.Contains([]byte(last.Content), []byte("workspace-content")) {
							t.Errorf("未读取工作区文件: %s", last.Content)
						}
					} else if !bytes.Contains([]byte(last.Content), []byte("error")) {
						t.Errorf("拒绝未返回结构化结果: %s", last.Content)
					}
					if input.Stream {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"最终回答\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"total_tokens\":7}}\n\ndata: [DONE]\n\n")
					} else {
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"最终回答"},"finish_reason":"stop"}],"usage":{"total_tokens":7}}`)
					}
					return
				}
				// 后续用户问题必须带上先前的工具轨迹；直接回答即可。
				if last.Content == "继续" {
					found := false
					for _, m := range input.Messages {
						if m.Role == "tool" {
							found = true
						}
					}
					if !found {
						t.Error("后续上下文丢失工具轨迹")
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"后续回答"},"finish_reason":"stop"}]}`)
					return
				}
				if input.Stream {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":%q},\"finish_reason\":null}]}\n\n", preamble)
					_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-read\",\"type\":\"function\",\"function\":{\"name\":\"read_file\",\"arguments\":\"{\\\"path\\\":\"}}]},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"a.txt\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"total_tokens\":3,\"vendor\":true}}\n\ndata: [DONE]\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"我会先读取文件，确认内容后再回答。","tool_calls":[{"id":"call-read","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"a.txt\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"total_tokens":3,"vendor":true}}`)
				}
			}))
			t.Cleanup(upstream.Close)
			cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
			if err != nil {
				t.Fatal(err)
			}
			cfg.MaxConcurrentRuns = 1
			if mode == "approve" || mode == "cancel" {
				cfg.CallTimeout = 250 * time.Millisecond
			}
			p := startApp(t, cfg, io.Discard)
			if mode == "never" {
				setApprovalPolicy(t, p, "never")
			} else if !automatic {
				setApprovalPolicy(t, p, "always")
			}
			mid := setupConversationModel(t, p, upstream.URL)
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("workspace-content"), 0600); err != nil {
				t.Fatal(err)
			}
			wid := resultString(t, p.call(t, "workspace.create", map[string]any{"name": "项目", "root_path": root}), "id")
			stream := mode == "automatic" || mode == "approve"
			accepted := p.call(t, "conversation.send", map[string]any{"model_id": mid, "workspace_id": wid, "stream": stream, "content": messageBlocks("读取文件")})
			runID, convID := resultString(t, accepted, "run_id"), resultString(t, accepted, "conversation_id")
			var approvalID string
			if !automatic {
				notification := p.event(t, "approval.requested")
				var payload struct {
					ApprovalID     string         `json:"approval_id"`
					ConversationID string         `json:"conversation_id"`
					Approval       agent.Approval `json:"approval"`
				}
				if err := json.Unmarshal(notification.Params, &payload); err != nil {
					t.Fatal(err)
				}
				if bytes.Contains(notification.Params, []byte(`"expires_at"`)) {
					t.Fatal("审批通知仍带过期字段")
				}
				approvalID = payload.ApprovalID
				if payload.ConversationID != convID {
					t.Fatal("审批事件丢失会话标识")
				}
				if payload.Approval.RequestHash == "" || !bytes.Contains(payload.Approval.Request, []byte("read_file")) {
					t.Fatal("未保存确定操作快照")
				}
				if toolResults.Load() != 0 {
					t.Fatal("审批前已经执行工具")
				}
				if mode == "approve" || mode == "cancel" {
					// 暂不作决定并等待超过单次模型超时，审批应保持 pending，之后仍可批准或取消。
					timer := time.NewTimer(350 * time.Millisecond)
					select {
					case <-timer.C:
					case <-t.Context().Done():
						t.Fatal(t.Context().Err())
					}
					timer.Stop()
					current := p.call(t, "approval.get", map[string]any{"id": approvalID})
					requireOK(t, current)
					var waiting agent.Approval
					if err := json.Unmarshal(current.Result, &waiting); err != nil || waiting.Status != agent.ApprovalPending || waiting.DecidedAt != nil {
						t.Fatalf("等待中的审批提前结束: %s %v", current.Result, err)
					}
					if toolResults.Load() != 0 {
						t.Fatal("等待期间执行了未批准操作")
					}
				}
				if mode == "approve" || mode == "reject" {
					decision := agent.ApprovalApproved
					if mode == "reject" {
						decision = agent.ApprovalRejected
					}
					params := map[string]any{"approval_id": approvalID, "decision": decision, "decision_note": "用户说明"}
					requireOK(t, p.call(t, "approval.decide", params))
					requireOK(t, p.call(t, "approval.decide", params))
					opposite := agent.ApprovalApproved
					if decision == agent.ApprovalApproved {
						opposite = agent.ApprovalRejected
					}
					if result := p.call(t, "approval.decide", map[string]any{"approval_id": approvalID, "decision": opposite}); result.Error == nil {
						t.Fatal("允许冲突改判")
					}
				}
				if mode == "cancel" {
					requireOK(t, p.call(t, "run.cancel", map[string]any{"run_id": runID}))
				}
			}
			terminal := "run.completed"
			if mode == "cancel" {
				terminal = "run.cancelled"
			}
			p.event(t, terminal)
			seen := p.seen.String()
			if intro, tool := strings.Index(seen, preamble), strings.Index(seen, `"type":"tool_call"`); intro < 0 || tool < intro {
				t.Fatal("执行前说明未在工具步骤之前发布")
			}
			if mode == "cancel" {
				if toolResults.Load() != 0 {
					t.Fatal("取消后执行了待审批工具")
				}
			} else if toolResults.Load() != 1 {
				t.Fatalf("工具结果次数错误: %d", toolResults.Load())
			}
			approvals := p.call(t, "approval.list", map[string]any{"run_id": runID})
			requireOK(t, approvals)
			var records []agent.Approval
			if err := json.Unmarshal(approvals.Result, &records); err != nil {
				t.Fatal(err)
			}
			if automatic {
				if len(records) != 0 {
					t.Fatal("自动放行伪造了用户审批")
				}
			} else {
				want := map[string]string{"approve": agent.ApprovalApproved, "reject": agent.ApprovalRejected, "cancel": agent.ApprovalCancelled}[mode]
				if len(records) != 1 || records[0].ID != approvalID || records[0].Status != want || records[0].DecidedAt == nil {
					t.Fatalf("审批终态错误: %+v", records)
				}
			}
			stepsFrame := p.call(t, "step.list", map[string]any{"run_id": runID})
			requireOK(t, stepsFrame)
			var steps []agent.StepSummary
			if err := json.Unmarshal(stepsFrame.Result, &steps); err != nil {
				t.Fatal(err)
			}
			modelCalls, toolCalls := 0, 0
			for _, step := range steps {
				if step.Type == "model_call" {
					modelCalls++
					if step.UsageBytes == 0 {
						t.Fatal("模型步骤丢失 usage")
					}
					if modelCalls == 1 {
						data := p.call(t, "step.data", map[string]any{"step_id": step.ID, "field": "output"})
						requireOK(t, data)
						var page struct {
							Fragment string
							Done     bool
						}
						if err := json.Unmarshal(data.Result, &page); err != nil || !page.Done {
							t.Fatalf("模型说明读取不完整: %s %v", data.Result, err)
						}
						var saved struct {
							Message model.Message `json:"context_message"`
						}
						if err := json.Unmarshal([]byte(page.Fragment), &saved); err != nil || saved.Message.Content != preamble || len(saved.Message.ToolCalls) != 1 {
							t.Fatalf("模型说明与工具调用未一起保存: %+v %v", saved, err)
						}
					}
				}
				if step.Type == "tool_call" {
					toolCalls++
					if step.ParentStepID == nil {
						t.Fatal("工具步骤无父模型步骤")
					}
				}
			}
			if toolCalls != 1 || modelCalls != map[bool]int{true: 1, false: 2}[mode == "cancel"] {
				t.Fatalf("步骤记录错误: %s", stepsFrame.Result)
			}
			if mode != "cancel" {
				rows := readMessages(t, p, convID)
				if len(rows) != 2 || len(rows[1].Content) != 1 || rows[1].Content[0].Text != "最终回答" {
					t.Fatal("过程说明混入最终回答或产生额外消息")
				}
			}
			if mode == "automatic" {
				accepted := p.call(t, "conversation.send", map[string]any{"conversation_id": convID, "stream": false, "content": messageBlocks("继续")})
				requireOK(t, accepted)
				p.event(t, "run.completed")
				messagesFrame := p.call(t, "conversation.messages", map[string]any{"conversation_id": convID})
				requireOK(t, messagesFrame)
				var rows []conversation.Message
				if err := json.Unmarshal(messagesFrame.Result, &rows); err != nil || len(rows) != 4 {
					t.Fatalf("消息与工具轨迹混存: %s %v", messagesFrame.Result, err)
				}
			}
			p.stop(t)
		})
	}
}

func TestAgentCompletesBeyondPreviousCallLimits(t *testing.T) {
	t.Parallel()
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				round := int(requests.Add(1))
				var input struct {
					Messages []struct {
						Role, Content string
						ToolCallID    string `json:"tool_call_id"`
					} `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				results := 0
				for _, message := range input.Messages {
					if message.Role != "tool" {
						continue
					}
					results++
					if message.ToolCallID == "" || !strings.Contains(message.Content, "workspace-content") {
						t.Error("实际文件读取结果未完整回传模型")
					}
				}
				if last := input.Messages[len(input.Messages)-1]; last.Role == "user" && last.Content == "继续任务" {
					if results != 48 {
						t.Errorf("后续请求未恢复完整工具历史: %d", results)
					}
					writeFileToolResponse(t, w, stream, map[string]any{"content": "已恢复全部工具历史"}, "stop")
					return
				}
				want := 0
				if round > 1 {
					want = 40 + round - 2
				}
				if results != want {
					t.Errorf("工具轨迹缺失: round=%d got=%d want=%d", round, results, want)
				}
				if round == 10 {
					writeFileToolResponse(t, w, stream, map[string]any{"content": "全部工具调用完成"}, "stop")
					return
				}
				count := 1
				if round == 1 {
					count = 40
				}
				calls := make([]any, count)
				for i := range count {
					call := map[string]any{"id": fmt.Sprintf("read-%d-%d", round, i), "type": "function", "function": map[string]string{"name": "read_file", "arguments": `{"path":"a.txt"}`}}
					if stream {
						call["index"] = i
					}
					calls[i] = call
				}
				writeFileToolResponse(t, w, stream, map[string]any{"tool_calls": calls}, "tool_calls")
			}))
			t.Cleanup(upstream.Close)
			cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
			if err != nil {
				t.Fatal(err)
			}
			cfg.MaxConcurrentRuns = 1
			p := startApp(t, cfg, io.Discard)
			mid := setupConversationModel(t, p, upstream.URL)
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("workspace-content"), 0600); err != nil {
				t.Fatal(err)
			}
			wid := resultString(t, p.call(t, "workspace.create", map[string]any{"name": "项目", "root_path": root}), "id")
			response := p.call(t, "conversation.send", map[string]any{"model_id": mid, "workspace_id": wid, "stream": stream, "content": messageBlocks("多轮读取并回答")})
			runID := resultString(t, response, "run_id")
			p.event(t, "run.completed")
			frame := p.call(t, "step.list", map[string]any{"run_id": runID, "limit": 200})
			requireOK(t, frame)
			var steps []agent.StepSummary
			if err := json.Unmarshal(frame.Result, &steps); err != nil {
				t.Fatal(err)
			}
			models, tools := 0, 0
			for _, step := range steps {
				if step.Status != agent.Completed {
					t.Fatalf("步骤未成功: %+v", step)
				}
				if step.Type == "model_call" {
					models++
					if step.UsageBytes == 0 {
						t.Fatal("模型实际 usage 丢失")
					}
				}
				if step.Type == "tool_call" {
					tools++
					if step.ParentStepID == nil || step.OutputBytes == 0 {
						t.Fatal("工具父步骤或结果丢失")
					}
				}
			}
			if requests.Load() != 10 || models != 10 || tools != 48 {
				t.Fatalf("长调用链未完整完成: requests=%d models=%d tools=%d", requests.Load(), models, tools)
			}
			requireOK(t, p.call(t, "conversation.send", map[string]any{"conversation_id": resultString(t, response, "conversation_id"), "stream": stream, "content": messageBlocks("继续任务")}))
			p.event(t, "run.completed")
			if requests.Load() != 11 {
				t.Fatal("后续请求重复执行了历史工具调用")
			}
			p.stop(t)
		})
	}
}

func TestAgentChecksLargeToolBatchBeforeExecuting(t *testing.T) {
	t.Parallel()
	for _, boundary := range []string{"bytes", "messages"} {
		t.Run(boundary, func(t *testing.T) {
			t.Parallel()
			args, count := `{}`, 300
			if boundary == "bytes" {
				args, count = `{"path":"`+strings.Repeat("x", 15900)+`"}`, 526
			}
			calls := make([]model.ToolCall, count)
			for i := range calls {
				calls[i] = model.ToolCall{ID: fmt.Sprintf("call-%d", i), Name: "operation", Arguments: []byte(args)}
			}
			encoded, err := json.Marshal([]model.Message{{Role: model.RoleAssistant, ToolCalls: calls}})
			if err != nil || (boundary == "bytes" && len(encoded) <= history.MaxBytes) {
				t.Fatalf("测试输入未越过上下文边界: %d %v", len(encoded), err)
			}
			rt, _ := runtimeFixture(t, 1)
			instance := &outcomeTestTool{path: filepath.Join(t.TempDir(), "evidence"), outcome: tool.OutcomeApplied}
			var requests atomic.Int32
			client := outcomeTestModel(func(context.Context, model.Request) (model.Response, error) {
				requests.Add(1)
				return model.Response{ToolCalls: calls, Output: []byte(`{"fixture":true}`)}, nil
			})
			id := policyTestID(t)
			service, _ := outcomeService(t, rt, instance, client, id, nil)
			sub, err := rt.Subscribe(events.Filter{})
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			accepted := sendOutcome(t, service, id, "", "检查上下文边界", nil)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			created, retained := 0, false
			for {
				event, err := sub.Next(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if event.RunID != accepted.Run.ID {
					continue
				}
				switch event.Type {
				case "step.created":
					step := event.Data.(agent.StepSummary)
					created++
					if step.Type != "model_call" {
						t.Fatal("越界检查前已受理工具操作")
					}
				case "step.updated":
					step := event.Data.(agent.StepSummary)
					// 更新事件由实际保存成功后发布；避免通过列表重新解码整份超大结果。
					retained = retained || (step.Type == "model_call" && step.Status == agent.Completed && step.OutputBytes > 0)
				case "run.completed", "run.cancelled":
					t.Fatalf("越界调用被当作成功或取消: %s", event.Type)
				case "run.failed":
					run := event.Data.(agent.Run)
					if run.ErrorKind == nil || *run.ErrorKind != "input_limit" {
						t.Fatalf("没有按上下文边界拒绝: %+v", run)
					}
					rt.manager.Wait(accepted.Run.ID)
					if created != 1 || !retained || requests.Load() != 1 || instance.calls.Load() != 0 {
						t.Fatal("越界前执行了工具，或模型结果未保存")
					}
					return
				}
			}
		})
	}
}
