package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/conversation/compaction"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/tokens"
	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/command"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem"
)

type compactModel struct {
	mode       atomic.Int32
	summaries  atomic.Int32
	blockAfter atomic.Int32
	mu         sync.Mutex
	answers    map[string]string
	started    chan struct{}
}

func compactionEndpoint(t *testing.T) (*compactModel, *httptest.Server) {
	t.Helper()
	m := &compactModel{answers: map[string]string{}, started: make(chan struct{}, 8)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Tools    []json.RawMessage                `json:"tools"`
			Messages []struct{ Role, Content string } `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Messages) == 0 {
			t.Errorf("模型请求损坏: %v", err)
			w.WriteHeader(400)
			return
		}
		text := strings.Repeat("已记录事实。", 50)
		if input.Messages[0].Role == "system" && input.Messages[0].Content == compaction.Prompt {
			if len(input.Tools) != 0 {
				t.Error("摘要模型获得工具")
			}
			m.summaries.Add(1)
			text = `{"goals":["继续完成任务"],"constraints":["保留用户约束"],"decisions":[],"facts":["历史操作需按审计核实"],"open_questions":[]}`
			switch m.mode.Load() {
			case 1:
				text = "不符合结构的摘要"
			case 3:
				large := strings.Repeat("x", 14000)
				content, err := json.Marshal(map[string][]string{"goals": {large}, "constraints": {large}, "decisions": {large}, "facts": {large}, "open_questions": {}})
				if err != nil {
					t.Error(err)
					return
				}
				text = string(content)
			case 2:
				if after := m.blockAfter.Load(); after > 0 && m.summaries.Load() < after {
					break
				}
				select {
				case m.started <- struct{}{}:
				default:
				}
				<-r.Context().Done()
				return
			}
		} else if len(input.Tools) == 0 {
			text = "会话标题"
		} else {
			last := input.Messages[len(input.Messages)-1].Content
			raw, err := json.Marshal(input.Messages)
			if err != nil {
				t.Error(err)
			}
			m.mu.Lock()
			m.answers[last] = string(raw)
			m.mu.Unlock()
		}
		raw, err := json.Marshal(text)
		if err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "compaction-fixture")
		_, _ = fmt.Fprintf(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":%s},"finish_reason":"stop"}],"usage":{"total_tokens":7,"fixture":true}}`, raw)
	}))
	t.Cleanup(server.Close)
	return m, server
}

func compactionSetting(t *testing.T, p *appPeer, cfg settings.Compaction) {
	t.Helper()
	value := p.call(t, "settings.get", map[string]any{"key": settings.CompactionKey})
	requireOK(t, value)
	var record settings.Record
	if err := json.Unmarshal(value.Result, &record); err != nil {
		t.Fatal(err)
	}
	requireOK(t, p.call(t, "settings.update", map[string]any{"key": settings.CompactionKey, "revision": record.Revision, "value": cfg}))
}

func compactSend(t *testing.T, p *appPeer, modelID, conversationID, text string) string {
	t.Helper()
	response := p.call(t, "conversation.send", map[string]any{"conversation_id": conversationID, "model_id": modelID, "stream": false, "content": messageBlocks(text)})
	id := resultString(t, response, "conversation_id")
	p.event(t, "run.completed")
	return id
}

func contextVersions(t *testing.T, p *appPeer, id string) []conversation.ContextVersion {
	t.Helper()
	response := p.call(t, "conversation.context_versions", map[string]any{"conversation_id": id, "limit": 200})
	requireOK(t, response)
	var rows []conversation.ContextVersion
	if err := json.Unmarshal(response.Result, &rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestCompactionManualFailureCancelAndRestartThroughStdio(t *testing.T) {
	upstream, server := compactionEndpoint(t)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	p := startApp(t, cfg, io.Discard)
	mid := setupConversationModel(t, p, server.URL)
	compression := settings.DefaultCompaction()
	compression.Enabled, compression.RetainTurns = false, 1
	compactionSetting(t, p, compression)
	cid := ""
	for i := range 3 {
		cid = compactSend(t, p, mid, cid, fmt.Sprintf("问题%d-保留约束-", i)+strings.Repeat("历史内容", 400))
	}
	before := readMessages(t, p, cid)
	response := p.call(t, "conversation.compact", map[string]any{"conversation_id": cid})
	runID := resultString(t, response, "run_id")
	p.event(t, "run.completed")
	versions := contextVersions(t, p, cid)
	if len(versions) != 1 || versions[0].Status != "active" || versions[0].ThroughSequence != 4 {
		t.Fatalf("摘要未激活: %+v", versions)
	}
	activeID := versions[0].ID
	detail := p.call(t, "conversation.context_version", map[string]any{"conversation_id": cid, "id": activeID})
	requireOK(t, detail)
	var active conversation.ContextVersion
	if err := json.Unmarshal(detail.Result, &active); err != nil {
		t.Fatal(err)
	}
	if len(active.Sources) != 4 || active.Summary == "" || active.Sources[0].Fingerprint == "" {
		t.Fatalf("摘要来源丢失: %+v", active)
	}
	if len(readMessages(t, p, cid)) != len(before) {
		t.Fatal("手动压缩创建或删除消息")
	}
	stepsFrame := p.call(t, "step.list", map[string]any{"run_id": runID})
	requireOK(t, stepsFrame)
	var steps []agent.StepSummary
	if err := json.Unmarshal(stepsFrame.Result, &steps); err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 || steps[0].Type != "context_compression" || steps[0].UsageBytes != 0 || steps[1].ParentStepID == nil || *steps[1].ParentStepID != steps[0].ID || steps[1].UsageBytes == 0 {
		t.Fatalf("摘要用量或父子关系错误: %s", stepsFrame.Result)
	}
	compactSend(t, p, mid, cid, "压缩后继续")
	upstream.mu.Lock()
	request := upstream.answers["压缩后继续"]
	upstream.mu.Unlock()
	if !strings.Contains(request, "conversation_context_summary") || !strings.Contains(request, "问题2") || strings.Contains(request, "问题0") {
		t.Fatalf("没有复用摘要及保留最新历史: %s", request)
	}
	for i, mode := range []int32{1, 3} {
		upstream.mode.Store(mode)
		resultString(t, p.call(t, "conversation.compact", map[string]any{"conversation_id": cid}), "run_id")
		p.event(t, "run.failed")
		versions = contextVersions(t, p, cid)
		if len(versions) != i+2 || versions[0].Status != "failed" || versions[len(versions)-1].ID != activeID || versions[len(versions)-1].Status != "active" {
			t.Fatalf("失败替换了有效摘要: %+v", versions)
		}
	}
	upstream.mode.Store(2)
	cancelled := resultString(t, p.call(t, "conversation.compact", map[string]any{"conversation_id": cid}), "run_id")
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case <-upstream.started:
	case <-timer.C:
		t.Fatal("摘要请求未开始")
	}
	busy := p.call(t, "conversation.compact", map[string]any{"conversation_id": cid})
	if busy.Error == nil || busy.Error.Code != 1006 {
		t.Fatal("允许同一会话并发压缩")
	}
	other := compactSend(t, p, mid, "", "压缩等待不阻塞其他会话")
	if other == cid {
		t.Fatal("独立会话标识混淆")
	}
	compression.RetainTurns = 2
	compactionSetting(t, p, compression)
	waiting := p.call(t, "run.get", map[string]any{"id": cancelled})
	requireOK(t, waiting)
	var waitingRun agent.Run
	if err := json.Unmarshal(waiting.Result, &waitingRun); err != nil {
		t.Fatal(err)
	}
	if waitingRun.SettingsSnapshot == nil || waitingRun.SettingsSnapshot.Compaction == nil || waitingRun.SettingsSnapshot.Compaction.RetainTurns != 1 {
		t.Fatal("全局设置改变了已受理压缩策略")
	}

	requireOK(t, p.call(t, "run.cancel", map[string]any{"run_id": cancelled}))
	p.event(t, "run.cancelled")
	versions = contextVersions(t, p, cid)
	if versions[0].Status != "failed" || versions[len(versions)-1].Status != "active" {
		t.Fatalf("取消留下半成品: %+v", versions)
	}
	upstream.mode.Store(0)
	p.stop(t)
	restarted := startApp(t, cfg, io.Discard)
	if rows := contextVersions(t, restarted, cid); len(rows) != 4 || rows[len(rows)-1].ID != activeID {
		t.Fatalf("重启丢失摘要: %+v", rows)
	}
	compactSend(t, restarted, mid, cid, "重启后继续")
	upstream.mu.Lock()
	request = upstream.answers["重启后继续"]
	upstream.mu.Unlock()
	if !strings.Contains(request, activeID) {
		t.Fatal("重启未复用有效摘要")
	}
}

func TestAutomaticCompactionSurvivesLongHistoryAndKeepsConversationsIsolated(t *testing.T) {
	t.Parallel()
	upstream, server := compactionEndpoint(t)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	p := startApp(t, cfg, io.Discard)
	mid := setupConversationModel(t, p, server.URL)
	compression := settings.DefaultCompaction()
	compression.TokenBudget, compression.RetainTurns = 25000, 1
	compactionSetting(t, p, compression)
	cid := ""
	// 129 轮产生 258 条消息，跨过原 256 条边界；数据库与模型端点均由本用例独占。
	for i := range 129 {
		cid = compactSend(t, p, mid, cid, fmt.Sprintf("长对话-%03d-", i)+strings.Repeat("必须保留目标。", 40))
	}
	versions := contextVersions(t, p, cid)
	if len(versions) < 2 || versions[0].Status != "active" || versions[0].BaseVersionID == nil || upstream.summaries.Load() < 2 {
		t.Fatalf("长对话未连续压缩: %+v", versions)
	}
	page := p.call(t, "conversation.messages", map[string]any{"conversation_id": cid, "limit": 200})
	requireOK(t, page)
	var messages []conversation.Message
	if err := json.Unmarshal(page.Result, &messages); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 200 || messages[len(messages)-1].Sequence != 258 {
		t.Fatal("长对话丢失原文或超过旧上限后未继续")
	}
	other := compactSend(t, p, mid, "", "独立会话问题")
	upstream.mu.Lock()
	request := upstream.answers["独立会话问题"]
	upstream.mu.Unlock()
	if strings.Contains(request, "conversation_context_summary") || strings.Contains(request, cid) || len(contextVersions(t, p, other)) != 0 {
		t.Fatal("摘要串入其他会话")
	}
}

func TestAutomaticCompactionTokenTriggerKeepsCurrentQuestion(t *testing.T) {
	upstream, server := compactionEndpoint(t)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	p := startApp(t, cfg, io.Discard)
	mid := setupConversationModel(t, p, server.URL)
	compression := settings.DefaultCompaction()
	// 从独立会话的真实请求取得固定提示开销，历史预算仍在第三轮触发压缩。
	compactSend(t, p, mid, "", "读取当前固定上下文")
	upstream.mu.Lock()
	probe := upstream.answers["读取当前固定上下文"]
	upstream.mu.Unlock()
	var requestMessages []model.Message
	if err := json.Unmarshal([]byte(probe), &requestMessages); err != nil {
		t.Fatal(err)
	}
	var fixed []model.Message
	for _, message := range requestMessages {
		if message.Role == model.RoleSystem {
			fixed = append(fixed, message)
		}
	}
	if len(fixed) != 1 || fixed[0].Content == "" {
		t.Fatal("主对话缺少固定过程沟通提示")
	}
	definitions := []model.ToolDefinition{agent.UserInputDefinition()}
	for _, instance := range append(filesystem.Tools(), command.Tools(nil)...) {
		definitions = append(definitions, instance.Definition().ToolDefinition)
	}
	compression.TokenBudget, compression.RetainTurns = tokens.Estimate(fixed, definitions, compression.ImageTokenEstimate)+5700, 1
	compactionSetting(t, p, compression)
	cid := ""
	for i := range 2 {
		cid = compactSend(t, p, mid, cid, fmt.Sprintf("较早问题%d-", i)+strings.Repeat("历史内容", 300))
	}
	cid = compactSend(t, p, mid, cid, "当前问题必须保持原样")
	if versions := contextVersions(t, p, cid); len(versions) != 1 || versions[0].ThroughSequence != 2 {
		t.Fatalf("Token 预算或保留轮次错误: %+v", versions)
	}
	upstream.mu.Lock()
	request := upstream.answers["当前问题必须保持原样"]
	upstream.mu.Unlock()
	if !strings.Contains(request, "当前问题必须保持原样") || !strings.Contains(request, "较早问题1") || strings.Contains(request, "较早问题0") {
		t.Fatal("Token 触发压缩吞掉了保留轮次或当前问题")
	}
	var afterCompression []model.Message
	if err := json.Unmarshal([]byte(request), &afterCompression); err != nil || len(afterCompression) == 0 || afterCompression[0].Role != model.RoleSystem || afterCompression[0].Content != fixed[0].Content {
		t.Fatal("历史压缩移除了主对话的过程沟通规则")
	}
	rows := readMessages(t, p, cid)
	user := rows[len(rows)-2]
	frame := p.call(t, "step.list", map[string]any{"run_id": user.RunID})
	requireOK(t, frame)
	var steps []agent.StepSummary
	if err := json.Unmarshal(frame.Result, &steps); err != nil {
		t.Fatal(err)
	}
	summaryStep := ""
	for _, step := range steps {
		if step.Name == "生成历史摘要" {
			summaryStep = step.ID
		}
	}
	if summaryStep == "" {
		t.Fatal("缺少摘要模型步骤")
	}
	response := p.call(t, "conversation.resend", map[string]any{"conversation_id": cid, "message_id": user.ID, "stream": false, "content": messageBlocks("修改后的当前问题")})
	requireOK(t, response)
	p.event(t, "run.completed")
	saved := p.call(t, "step.get", map[string]any{"id": summaryStep})
	requireOK(t, saved)
	var step agent.StepSummary
	if err := json.Unmarshal(saved.Result, &step); err != nil {
		t.Fatal(err)
	}
	if step.OutputBytes == 0 || step.UsageBytes == 0 {
		t.Fatal("重发清除了仍有效摘要的模型结果或 usage")
	}

}

func TestManualCompactionWithoutEarlierHistoryIsAnExplicitNoop(t *testing.T) {
	upstream, server := compactionEndpoint(t)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	p := startApp(t, cfg, io.Discard)
	mid := setupConversationModel(t, p, server.URL)
	cid := compactSend(t, p, mid, "", "刚开始的对话")
	run := resultString(t, p.call(t, "conversation.compact", map[string]any{"conversation_id": cid}), "run_id")
	p.event(t, "run.completed")
	if upstream.summaries.Load() != 0 || len(contextVersions(t, p, cid)) != 0 || len(readMessages(t, p, cid)) != 2 {
		t.Fatal("无可压缩历史时发起模型调用或改变消息")
	}
	steps := p.call(t, "step.list", map[string]any{"run_id": run})
	requireOK(t, steps)
	var rows []agent.StepSummary
	if err := json.Unmarshal(steps.Result, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Type != "context_compression" {
		t.Fatal("无操作压缩错误地创建模型步骤")
	}
	output := p.call(t, "step.data", map[string]any{"step_id": rows[0].ID, "field": "output"})
	requireOK(t, output)
	if !strings.Contains(string(output.Result), "no_earlier_complete_turns") {
		t.Fatal("缺少无操作说明")
	}
}

func manyBatchHistory(t *testing.T, p *appPeer, modelID string) string {
	t.Helper()
	compression := settings.DefaultCompaction()
	compression.Enabled, compression.RetainTurns = false, 1
	compactionSetting(t, p, compression)
	cid := ""
	for i := range 11 {
		cid = compactSend(t, p, modelID, cid, fmt.Sprintf("历史问题%d-", i)+strings.Repeat("历史内容", 300))
	}
	return compactSend(t, p, modelID, cid, "最近轮次必须保留")
}

func TestCompactionContinuesBeyondPreviousBatchLimit(t *testing.T) {
	t.Parallel()
	for _, automatic := range []bool{false, true} {
		t.Run(fmt.Sprint(automatic), func(t *testing.T) {
			t.Parallel()
			upstream, server := compactionEndpoint(t)
			cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
			if err != nil {
				t.Fatal(err)
			}
			cfg.MaxConcurrentRuns = 1
			p := startApp(t, cfg, io.Discard)
			mid := setupConversationModel(t, p, server.URL)
			cid := manyBatchHistory(t, p, mid)
			compression := settings.DefaultCompaction()
			compression.Enabled, compression.TokenBudget, compression.RetainTurns = automatic, 5000, 1
			compactionSetting(t, p, compression)
			method := "conversation.compact"
			params := map[string]any{"conversation_id": cid}
			if automatic {
				method = "conversation.send"
				params["stream"] = false
				params["content"] = messageBlocks("当前问题必须保留")
			}
			runID := resultString(t, p.call(t, method, params), "run_id")
			p.event(t, "run.completed")
			versions := contextVersions(t, p, cid)
			if len(versions) != 11 || versions[0].Status != "active" || versions[0].ThroughSequence != 22 || upstream.summaries.Load() != 11 {
				t.Fatalf("超过八批后未继续或范围错误: %+v calls=%d", versions, upstream.summaries.Load())
			}
			for i, version := range versions {
				if i > 0 && version.Status != "superseded" {
					t.Fatalf("摘要版本未原子切换: %+v", version)
				}
			}
			frame := p.call(t, "step.list", map[string]any{"run_id": runID, "limit": 200})
			requireOK(t, frame)
			var steps []agent.StepSummary
			if err := json.Unmarshal(frame.Result, &steps); err != nil {
				t.Fatal(err)
			}
			parents, children := 0, 0
			for _, step := range steps {
				if step.Status != agent.Completed {
					t.Fatalf("压缩步骤未完成: %+v", step)
				}
				if step.Type == "context_compression" {
					parents++
				}
				if step.Name == "生成历史摘要" {
					children++
					if step.ParentStepID == nil || step.UsageBytes == 0 {
						t.Fatal("摘要子步骤或实际 usage 丢失")
					}
				}
			}
			if parents != 11 || children != 11 {
				t.Fatalf("批次步骤未完整保存: %d %d", parents, children)
			}
			wantMessages := 24
			if automatic {
				wantMessages = 26
			}
			if len(readMessages(t, p, cid)) != wantMessages {
				t.Fatal("多批压缩删除了原文或创建了额外消息")
			}
			p.stop(t)
		})
	}
}

func TestCompactionCanCancelBeyondPreviousBatchLimit(t *testing.T) {
	t.Parallel()
	upstream, server := compactionEndpoint(t)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConcurrentRuns = 1
	p := startApp(t, cfg, io.Discard)
	mid := setupConversationModel(t, p, server.URL)
	cid := manyBatchHistory(t, p, mid)
	compression := settings.DefaultCompaction()
	compression.Enabled, compression.TokenBudget, compression.RetainTurns = false, 5000, 1
	compactionSetting(t, p, compression)
	upstream.mode.Store(2)
	upstream.blockAfter.Store(10)
	runID := resultString(t, p.call(t, "conversation.compact", map[string]any{"conversation_id": cid}), "run_id")
	children := 0
	for children < 10 {
		frame := p.read(t)
		p.pending = append(p.pending, frame)
		if frame.Method == "run.failed" || frame.Method == "run.completed" {
			t.Fatalf("压缩提前结束: %s", frame.Params)
		}
		if frame.Method == "step.created" {
			var value struct {
				Step agent.StepSummary `json:"step"`
			}
			if err := json.Unmarshal(frame.Params, &value); err != nil {
				t.Fatal(err)
			}
			if value.Step.Name == "生成历史摘要" {
				children++
			}
		}
	}
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case <-upstream.started:
	case <-timer.C:
		t.Fatal("第十批摘要没有开始")
	}
	requireOK(t, p.call(t, "run.cancel", map[string]any{"run_id": runID}))
	p.event(t, "run.cancelled")
	versions := contextVersions(t, p, cid)
	if len(versions) != 10 || versions[0].Status != "failed" || versions[1].Status != "active" || versions[1].ThroughSequence != 18 {
		t.Fatalf("取消损坏已提交批次: %+v", versions)
	}
	upstream.mode.Store(0)
	resultString(t, p.call(t, "conversation.compact", map[string]any{"conversation_id": cid}), "run_id")
	p.event(t, "run.completed")
	if versions := contextVersions(t, p, cid); len(versions) != 12 || versions[0].Status != "active" || versions[0].ThroughSequence != 22 {
		t.Fatalf("取消后不能继续压缩: %+v", versions)
	}
	p.stop(t)
}
