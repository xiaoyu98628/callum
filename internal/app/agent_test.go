package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/conversation"
)

func TestAgentToolsAndApprovalThroughStdio(t *testing.T) {
	for _, mode := range []string{"automatic", "approve", "reject", "cancel"} {
		t.Run(mode, func(t *testing.T) {
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
				if len(input.Tools) != 3 {
					t.Errorf("未提供三个工具定义: %d", len(input.Tools))
				}
				last := input.Messages[len(input.Messages)-1]
				if last.Role == "tool" {
					toolResults.Add(1)
					if last.ToolCallID != "call-read" || len(input.Messages) < 3 || len(input.Messages[len(input.Messages)-2].Calls) != 1 {
						t.Error("工具调用与结果未配对")
					}
					if mode == "automatic" || mode == "approve" {
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
					_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-read\",\"type\":\"function\",\"function\":{\"name\":\"read_file\",\"arguments\":\"{\\\"path\\\":\"}}]},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"a.txt\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"total_tokens\":3,\"vendor\":true}}\n\ndata: [DONE]\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call-read","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"a.txt\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"total_tokens":3,"vendor":true}}`)
				}
			}))
			t.Cleanup(upstream.Close)
			cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
			if err != nil {
				t.Fatal(err)
			}
			cfg.MaxConcurrentRuns = 1
			if mode != "automatic" {
				cfg.ApprovalMode = "always"
			}
			if mode == "approve" || mode == "cancel" {
				cfg.CallTimeout = 250 * time.Millisecond
			}
			p := startApp(t, cfg, io.Discard)
			mid := setupConversationModel(t, p, upstream.URL)
			frame := p.call(t, "model.get", map[string]any{"id": mid})
			requireOK(t, frame)
			var modelValue map[string]any
			if err := json.Unmarshal(frame.Result, &modelValue); err != nil {
				t.Fatal(err)
			}
			modelValue["capabilities"], modelValue["model_type"] = map[string]bool{"tools": true}, []string{"tools"}
			// 更新接口只接受配置字段。
			update := map[string]any{"id": mid, "provider_id": modelValue["provider_id"], "display_name": "工具模型", "remote_model_id": "remote-model", "enabled": true, "capabilities": modelValue["capabilities"], "model_type": modelValue["model_type"]}
			requireOK(t, p.call(t, "model.update", update))
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("workspace-content"), 0600); err != nil {
				t.Fatal(err)
			}
			wid := resultString(t, p.call(t, "workspace.create", map[string]any{"name": "项目", "root_path": root}), "id")
			stream := mode == "automatic" || mode == "approve"
			accepted := p.call(t, "conversation.send", map[string]any{"model_id": mid, "workspace_id": wid, "stream": stream, "content": messageBlocks("读取文件")})
			runID, convID := resultString(t, accepted, "run_id"), resultString(t, accepted, "conversation_id")
			var approvalID string
			if mode != "automatic" {
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
			if mode == "automatic" {
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
