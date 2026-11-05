package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/settings"
)

func TestTokenBudgetMissingOrUncompressibleInputDoesNotCallModel(t *testing.T) {
	upstream, server := compactionEndpoint(t)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	p := startApp(t, cfg, io.Discard)
	pid := resultID(t, p.call(t, "provider.create", map[string]any{"name": "未知容量", "protocol": "chatcompletions", "base_url": server.URL + "/v1"}), "id")
	mid := resultID(t, p.call(t, "model.create", map[string]any{"provider_id": pid, "display_name": "模型", "remote_model_id": "unknown", "enabled": true}), "id")
	for i, kind := range []string{"token_budget_unconfigured", "token_budget_exceeded"} {
		if i == 1 {
			value := settings.DefaultCompaction()
			value.TokenBudget = 100
			compactionSetting(t, p, value)
		}
		requireOK(t, p.call(t, "conversation.send", map[string]any{"model_id": mid, "stream": false, "content": messageBlocks("当前必须保留的问题" + strings.Repeat("代码", 100))}))
		frame := p.event(t, "run.failed")
		var payload struct {
			Run agent.Run `json:"run"`
		}
		if err := json.Unmarshal(frame.Params, &payload); err != nil || payload.Run.ErrorKind == nil || *payload.Run.ErrorKind != kind {
			t.Fatalf("预算错误未明确报告: %s %v", frame.Params, err)
		}
	}
	upstream.mu.Lock()
	count := len(upstream.answers)
	upstream.mu.Unlock()
	if count != 0 || upstream.summaries.Load() != 0 {
		t.Fatal("无效预算仍调用了模型")
	}
	value := settings.DefaultCompaction()
	value.TokenBudget = 50000
	compactionSetting(t, p, value)
	compactSend(t, p, mid, "", "未知容量使用显式预算")
}

func TestShortHistoryDoesNotCompressAtOldMessageThreshold(t *testing.T) {
	upstream, server := compactionEndpoint(t)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	p := startApp(t, cfg, io.Discard)
	mid := setupConversationModel(t, p, server.URL)
	compression := settings.DefaultCompaction()
	compression.TokenBudget = 100000
	compactionSetting(t, p, compression)
	cid := ""
	for range 65 {
		cid = compactSend(t, p, mid, cid, "简短问题")
	}
	if upstream.summaries.Load() != 0 || len(contextVersions(t, p, cid)) != 0 {
		t.Fatal("低于 Token 预算的历史仍按旧消息条数压缩")
	}
	rows := readMessages(t, p, cid)
	input := stepInput(t, p, rows[len(rows)-2].RunID)
	if !strings.Contains(input, `"estimation_method":"unicode-v1"`) || !strings.Contains(input, `"input_limit":100000`) {
		t.Fatalf("没有保存预算审计: %s", input)
	}
}

func TestActualPromptTokensAffectNextToolRoundWithoutChangingUsage(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call\",\"type\":\"function\",\"function\":{\"name\":\"missing_tool\",\"arguments\":\"{}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":20000,\"total_tokens\":20001}}\n\ndata: [DONE]\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"call","type":"function","function":{"name":"missing_tool","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":20000,"total_tokens":20001}}`)
				}
			}))
			t.Cleanup(server.Close)
			cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
			if err != nil {
				t.Fatal(err)
			}
			cfg.MaxConcurrentRuns = 1 // 不启动并行标题，精确检查回答模型调用次数。
			p := startApp(t, cfg, io.Discard)
			mid := setupConversationModel(t, p, server.URL)
			compression := settings.DefaultCompaction()
			compression.TokenBudget = 10000
			compactionSetting(t, p, compression)
			frame := p.call(t, "conversation.send", map[string]any{"model_id": mid, "stream": stream, "content": messageBlocks("调用工具")})
			runID := resultString(t, frame, "run_id")
			failed := p.event(t, "run.failed")
			if calls.Load() != 1 || !strings.Contains(string(failed.Params), "token_budget_exceeded") {
				t.Fatalf("真实输入用量未影响下一轮: calls=%d %s", calls.Load(), failed.Params)
			}
			listed := p.call(t, "step.list", map[string]any{"run_id": runID})
			requireOK(t, listed)
			var steps []agent.StepSummary
			if err := json.Unmarshal(listed.Result, &steps); err != nil {
				t.Fatal(err)
			}
			if len(steps) != 2 {
				t.Fatalf("工具事实未保留或错误增加模型步骤: %+v", steps)
			}
			usage := p.call(t, "step.data", map[string]any{"step_id": steps[0].ID, "field": "usage"})
			requireOK(t, usage)
			var data struct {
				Fragment string `json:"fragment"`
			}
			if err := json.Unmarshal(usage.Result, &data); err != nil || data.Fragment != `{"prompt_tokens":20000,"total_tokens":20001}` {
				t.Fatalf("实际 usage 被估算改写: %s %v", usage.Result, err)
			}
		})
	}
}

// stepInput 通过公开 stdio 数据接口读取审计，而不是依赖测试进程中的估算器状态。
func stepInput(t *testing.T, p *appPeer, runID string) string {
	t.Helper()
	frame := p.call(t, "step.list", map[string]any{"run_id": runID})
	requireOK(t, frame)
	var steps []agent.StepSummary
	if err := json.Unmarshal(frame.Result, &steps); err != nil {
		t.Fatal(err)
	}
	for _, step := range steps {
		if step.Type != "model_call" || step.Name != "生成回答" {
			continue
		}
		data := p.call(t, "step.data", map[string]any{"step_id": step.ID, "field": "input"})
		requireOK(t, data)
		var result struct {
			Fragment string `json:"fragment"`
		}
		if err := json.Unmarshal(data.Result, &result); err != nil {
			t.Fatal(err)
		}
		return result.Fragment
	}
	t.Fatal("缺少回答模型步骤")
	return ""
}
