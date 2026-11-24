package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/events"
	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/conversation/history"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/tool"
)

func TestStartFollowUpRejectsNilCallbackBeforeCommit(t *testing.T) {
	rt, _ := runtimeFixture(t, 1)
	ctx := t.Context()
	modelID := directModel(t, rt, "https://example.test")
	svc := rt.Conversations()
	var queued conversation.FollowUp
	err := svc.Send(ctx, conversation.Send{ModelID: modelID, Content: []model.Block{{Type: "text", Text: "当前任务"}}}, func(ex conversation.Execution) error {
		var err error
		queued, err = svc.AddFollowUp(ctx, conversation.FollowUpInput{
			ConversationID: ex.Conversation.ID, ExpectedRunID: ex.Run.ID, Mode: "queue",
			Content: []model.Block{{Type: "text", Text: "稍后发送"}},
		})
		if !rt.Cancel(ex.Run.ID) {
			t.Fatal("受理阶段无法取消任务")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	rt.manager.Wait(queued.ExpectedRunID)
	request := conversation.FollowUpStart{ConversationID: queued.ConversationID, ID: queued.ID, Revision: queued.Revision}
	if err := svc.StartFollowUp(ctx, request, nil); !errors.Is(err, conversation.ErrInvalid) {
		t.Fatalf("缺失回调未返回参数错误: %v", err)
	}
	items, err := svc.FollowUps(ctx, queued.ConversationID)
	if err != nil || len(items) != 1 || items[0].ID != queued.ID || items[0].Status != queued.Status || items[0].Revision != queued.Revision {
		t.Fatalf("拒绝后队列被改写: %+v %v", items, err)
	}
	page, err := svc.History(ctx, queued.ConversationID, "", 25)
	if err != nil || len(page.Runs) != 1 || page.Runs[0].ID != queued.ExpectedRunID || page.Runs[0].Status != agent.Cancelled {
		t.Fatalf("拒绝后执行记录发生变化: %+v %v", page.Runs, err)
	}
}

func TestSteeringAcceptedBeforeExecutionStarts(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		t.Run(fmt.Sprintf("rejected=%t", rejected), func(t *testing.T) {
			rt, _ := runtimeFixture(t, 1)
			instance := &outcomeTestTool{path: filepath.Join(t.TempDir(), "unused")}
			var sampled atomic.Bool
			client := outcomeTestModel(func(_ context.Context, req model.Request) (model.Response, error) {
				last := req.Messages[len(req.Messages)-1]
				if last.Role != model.RoleUser || !strings.Contains(history.Text(last.Blocks), "开跑前引导") {
					return model.Response{}, errors.New("首个模型请求缺少启动前的引导")
				}
				sampled.Store(true)
				return model.Response{Text: "按引导完成"}, nil
			})
			id := policyTestID(t)
			svc, _ := outcomeService(t, rt, instance, client, id, nil)
			sub, err := rt.Subscribe(events.Filter{})
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			var ex conversation.Execution
			var guide conversation.FollowUp
			rejection := errors.New("受理响应不可写")
			err = svc.Send(t.Context(), conversation.Send{ModelID: id, Stream: new(false), Content: []model.Block{{Type: "text", Text: "原始任务"}}}, func(accepted conversation.Execution) error {
				ex = accepted
				var err error
				guide, err = svc.AddFollowUp(t.Context(), conversation.FollowUpInput{
					ConversationID: ex.Conversation.ID, ExpectedRunID: ex.Run.ID, Mode: "steer",
					Content: []model.Block{{Type: "text", Text: "开跑前引导"}},
				})
				if err != nil || guide.Receipt == nil {
					t.Fatalf("启动前引导未被受理: %v", err)
				}
				if rejected {
					return rejection
				}
				return nil
			})
			if rejected {
				if !errors.Is(err, rejection) || sampled.Load() {
					t.Fatalf("受理响应失败后仍启动执行或未保留原错误: %v", err)
				}
				requeued := nextBusiness(t, sub, "conversation.steering_requeued", ex.Run.ID)
				items, err := svc.FollowUps(t.Context(), ex.Conversation.ID)
				if err != nil || len(items) != 1 || items[0].ID != guide.ID || items[0].Status != conversation.FollowUpQueued ||
					len(requeued.RemovedMessageIDs) != 1 || requeued.RemovedMessageIDs[0] != guide.ID {
					t.Fatalf("启动失败后未回收引导及临时消息: %+v %+v %v", items, requeued, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if terminal := awaitOutcome(t, rt, sub, ex.Run.ID, nil); terminal.Type != "run.completed" || !sampled.Load() {
				t.Fatalf("执行未使用启动前引导完成: %+v", terminal)
			}
		})
	}
}

func TestSteeringLetsStartedToolFinishAndSkipsRemainingCall(t *testing.T) {
	rt, _ := runtimeFixture(t, 1)
	started, release := make(chan struct{}), make(chan struct{})
	signalStarted := sync.OnceFunc(func() { close(started) })
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	instance := &outcomeTestTool{path: filepath.Join(t.TempDir(), "applied"), outcome: tool.OutcomeApplied, after: func() {
		signalStarted()
		<-release
	}}
	client := outcomeTestModel(func(_ context.Context, req model.Request) (model.Response, error) {
		last := req.Messages[len(req.Messages)-1]
		if !strings.Contains(history.Text(last.Blocks), "跳过剩余旧工具") {
			return model.Response{ToolCalls: []model.ToolCall{
				{ID: "started", Name: "operation", Arguments: json.RawMessage(`{}`)},
				{ID: "remaining", Name: "operation", Arguments: json.RawMessage(`{}`)},
			}}, nil
		}
		var applied, skipped bool
		for _, message := range req.Messages {
			applied = applied || (message.ToolCallID == "started" && strings.Contains(message.Content, `"outcome":"applied"`))
			skipped = skipped || (message.ToolCallID == "remaining" && strings.Contains(message.Content, `"kind":"steered"`))
		}
		if !applied || !skipped {
			return model.Response{}, errors.New("引导后的模型请求缺少完整工具轨迹")
		}
		return model.Response{Text: "按引导完成"}, nil
	})
	id := policyTestID(t)
	svc, _ := outcomeService(t, rt, instance, client, id, nil)
	sub, err := rt.Subscribe(events.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	ex := sendOutcome(t, svc, id, "", "执行两项操作", nil)
	waitSignal(t, started)
	if _, err := svc.AddFollowUp(t.Context(), conversation.FollowUpInput{
		ConversationID: ex.Conversation.ID, ExpectedRunID: ex.Run.ID, Mode: "steer",
		Content: []model.Block{{Type: "text", Text: "跳过剩余旧工具"}},
	}); err != nil {
		t.Fatal(err)
	}
	unblock()
	if terminal := awaitOutcome(t, rt, sub, ex.Run.ID, nil); terminal.Type != "run.completed" || instance.calls.Load() != 1 {
		t.Fatalf("工具引导边界错误，终态 %+v，实际工具调用次数 %d", terminal, instance.calls.Load())
	}
}

func TestFollowUpQueueAndSteerThroughStdio(t *testing.T) {
	started, cancelled := make(chan struct{}), make(chan struct{})
	replacement, finish := make(chan struct{}), make(chan struct{})
	var guided, restored atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Stream   bool
			Tools    []json.RawMessage
			Messages []struct{ Role, Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Messages) == 0 {
			t.Errorf("模型请求无效: %v", err)
			return
		}
		last := input.Messages[len(input.Messages)-1]
		if len(input.Tools) > 0 {
			if !strings.Contains(input.Messages[0].Content, "Active task continuity") {
				t.Errorf("模型请求缺少任务连续性提示")
			}
			switch {
			case last.Content == "开始任务":
				close(started)
				<-r.Context().Done()
				close(cancelled)
				return
			case strings.Contains(last.Content, "改用新要求"):
				if !strings.Contains(last.Content, "较新的明确要求") {
					t.Error("引导缺少新要求优先的指令")
				}
				var original bool
				var steerCount int
				for _, m := range input.Messages {
					original = original || (m.Role == "user" && m.Content == "开始任务")
					if strings.Contains(m.Content, "改用新要求") {
						steerCount++
					}
				}
				guided.Store(original && steerCount == 1)
				close(replacement)
				select {
				case <-finish:
				case <-r.Context().Done():
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"按新要求完成"},"finish_reason":"stop"}]}`)
				return
			case strings.Contains(last.Content, "调整后的队列消息"):
				for _, m := range input.Messages {
					if m.Role == "user" && m.Content == "改用新要求" {
						restored.Store(true)
					}
				}
			}
		}
		if input.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"已处理\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"已处理"},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(upstream.Close)
	runtime, _ := runtimeFixture(t, 2)
	modelID := directModel(t, runtime, upstream.URL)
	p := sharedPeer(t, runtime)
	response := p.call(t, "conversation.send", map[string]any{"model_id": modelID, "stream": false, "content": messageBlocks("开始任务")})
	id, runID := resultString(t, response, "conversation_id"), resultString(t, response, "run_id")
	waitSignal(t, started)
	p.event(t, "message.created")
	p.event(t, "message.created")
	add := func(body string) string {
		t.Helper()
		return resultString(t, p.call(t, "conversation.follow_up_add", map[string]any{
			"conversation_id": id, "expected_run_id": runID, "mode": "queue", "content": messageBlocks(body),
		}), "id")
	}
	guide, queued, removed := add("改用新要求"), add("稍后处理"), add("需要移除")
	for _, action := range []map[string]any{
		{"id": queued, "action": "edit", "text": "调整后的队列消息"},
		{"id": removed, "action": "remove"},
		{"id": guide, "action": "steer", "expected_run_id": runID},
	} {
		action["conversation_id"], action["revision"] = id, 1
		requireOK(t, p.call(t, "conversation.follow_up_change", action))
	}
	waitSignal(t, cancelled)
	waitSignal(t, replacement)
	var receipt struct{ Message conversation.Message }
	event := p.event(t, "message.created")
	if err := json.Unmarshal(event.Params, &receipt); err != nil || receipt.Message.ID != guide || receipt.Message.RunID != runID || receipt.Message.Status != "running" || receipt.Message.Content[0].Text != "改用新要求" || receipt.Message.SteeringAfterStep == nil {
		t.Fatalf("引导未生成当前会话中的原文回执: %s, %v", event.Params, err)
	}
	var steps []agent.Step
	stepsFrame := p.call(t, "step.list", map[string]any{"run_id": runID})
	requireOK(t, stepsFrame)
	var oldStep, newStep int64
	if err := json.Unmarshal(stepsFrame.Result, &steps); err != nil {
		t.Fatal(err)
	}
	for _, step := range steps {
		if step.Type != "model_call" {
			continue
		}
		if step.ErrorKind != nil && *step.ErrorKind == "steered" {
			oldStep = step.Sequence
		}
		if strings.Contains(string(step.Input), `"steering_ids":["`+guide+`"]`) {
			newStep = step.Sequence
		}
	}
	if oldStep == 0 || newStep <= oldStep || *receipt.Message.SteeringAfterStep < oldStep || *receipt.Message.SteeringAfterStep >= newStep {
		t.Fatalf("引导没有位于切换前后的模型步骤之间: receipt=%s steps=%s", event.Params, stepsFrame.Result)
	}
	close(finish)
	p.event(t, "run.completed")
	history := p.call(t, "conversation.history", map[string]any{"conversation_id": id})
	requireOK(t, history)
	var page conversation.HistoryPage
	if err := json.Unmarshal(history.Result, &page); err != nil || len(page.Runs) != 1 || len(page.Messages) != 3 {
		t.Fatalf("同一轮历史缺少引导或回答: %s, %v", history.Result, err)
	}
	steered, answer := page.Messages[1], page.Messages[2]
	if steered.ID != guide || steered.Status != "completed" || steered.SteeringOrder < 1 || steered.SteeringAfterStep == nil || *steered.SteeringAfterStep != *receipt.Message.SteeringAfterStep || answer.Content[0].Text != "按新要求完成" {
		t.Fatalf("历史顺序、引导状态或调整后的回答不符: %s", history.Result)
	}
	if !guided.Load() {
		t.Fatal("引导未进入同一执行的下一模型请求，或旧工具没有被跳过")
	}
	response = p.call(t, "conversation.follow_ups", map[string]any{"conversation_id": id})
	requireOK(t, response)
	var items []conversation.FollowUp
	if err := json.Unmarshal(response.Result, &items); err != nil || len(items) != 1 || items[0].ID != queued || items[0].Revision != 2 {
		t.Fatalf("队列状态错误: %s, %v", response.Result, err)
	}
	request := map[string]any{"conversation_id": id, "id": queued, "revision": 1, "after_run_id": runID}
	if got := p.call(t, "conversation.follow_up_start", request); got.Error == nil {
		t.Fatal("旧队列版本被受理")
	}
	request["revision"] = 2
	response = p.call(t, "conversation.follow_up_start", request)
	if resultString(t, response, "run_id") == runID {
		t.Fatal("排队消息未开启新执行")
	}
	p.event(t, "run.completed")
	if !restored.Load() {
		t.Fatal("新执行上下文未恢复引导消息")
	}
	response = p.call(t, "conversation.follow_ups", map[string]any{"conversation_id": id})
	requireOK(t, response)
	if string(response.Result) != "[]" {
		t.Fatalf("已发送项目仍留在待发队列: %s", response.Result)
	}
}

func TestFollowUpsSurviveServiceRestart(t *testing.T) {
	started := make(chan struct{})
	var first sync.Once
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct{ Tools []json.RawMessage }
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Errorf("模型请求无效: %v", err)
			return
		}
		if len(input.Tools) != 0 {
			first.Do(func() { close(started) })
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"标题"},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	p := startApp(t, cfg, io.Discard)
	modelID := setupConversationModel(t, p, upstream.URL)
	result := p.call(t, "conversation.send", map[string]any{
		"model_id": modelID, "stream": false, "content": messageBlocks("执行中"),
	})
	conversationID, runID := resultString(t, result, "conversation_id"), resultString(t, result, "run_id")
	waitSignal(t, started)

	path := filepath.Join(t.TempDir(), "document.txt")
	original := []byte("原始附件")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	queued := resultString(t, p.call(t, "conversation.follow_up_add", map[string]any{
		"conversation_id": conversationID, "expected_run_id": runID, "mode": "queue",
		"content": []map[string]string{{"type": "file", "path": path, "name": "document.txt"}},
	}), "id")
	steer := resultString(t, p.call(t, "conversation.follow_up_add", map[string]any{
		"conversation_id": conversationID, "expected_run_id": runID, "mode": "steer", "content": messageBlocks("待用引导"),
	}), "id")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	p.stop(t)

	reopened := startApp(t, cfg, io.Discard)
	result = reopened.call(t, "conversation.follow_ups", map[string]any{"conversation_id": conversationID})
	requireOK(t, result)
	var items []conversation.FollowUp
	err = json.Unmarshal(result.Result, &items)
	if err != nil || len(items) != 2 || items[0].ID != queued || items[1].ID != steer || items[1].Status != conversation.FollowUpQueued {
		t.Fatalf("停机后队列或引导未恢复: %s, %v", result.Result, err)
	}
	ref := items[0].Content[0].AttachmentRef
	preserved, err := os.ReadFile(filepath.Join(cfg.DataDir, "attachments", filepath.FromSlash(ref)))
	if err != nil || string(preserved) != string(original) {
		t.Fatalf("托管附件未保留: %q, %v", preserved, err)
	}
}
