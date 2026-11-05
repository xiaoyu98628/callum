package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/events"
	agentservice "github.com/xiaoyu98628/callum/internal/agent/service"
	"github.com/xiaoyu98628/callum/internal/conversation"
	conversationservice "github.com/xiaoyu98628/callum/internal/conversation/service"
	"github.com/xiaoyu98628/callum/internal/model"
	agentstore "github.com/xiaoyu98628/callum/internal/storage/sqlite/agent"
	conversationstore "github.com/xiaoyu98628/callum/internal/storage/sqlite/conversation"
	"github.com/xiaoyu98628/callum/internal/tool"
	toolexecutor "github.com/xiaoyu98628/callum/internal/tool/executor"
	"github.com/xiaoyu98628/callum/internal/tool/policy"
	"github.com/xiaoyu98628/callum/internal/tool/registry"
)

// outcomeTestTool 只修改 t.TempDir 中的证据文件，用于验证真实副作用与状态分离。
// 这是受控测试工具，不会注册到产品运行时。
type outcomeTestTool struct {
	path     string
	outcome  tool.Outcome
	after    func()
	closeErr error
	calls    atomic.Int32
}

func (f *outcomeTestTool) Definition() tool.Definition {
	return tool.Definition{Name: "operation", Description: "操作事实验证", Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`), Source: "test", Version: "1", Effects: []tool.Effect{tool.EffectWrite}}
}
func (f *outcomeTestTool) Validate(args json.RawMessage) (json.RawMessage, error) {
	if string(args) != "{}" {
		return nil, tool.ErrInvalid
	}
	return json.RawMessage(`{}`), nil
}
func (f *outcomeTestTool) Prepare(context.Context, tool.Scope, json.RawMessage) (tool.Invocation, error) {
	return f, nil
}
func (f *outcomeTestTool) Target() *tool.Target { return nil }
func (f *outcomeTestTool) Close() error         { return f.closeErr }
func (f *outcomeTestTool) Execute(ctx context.Context) (tool.Result, error) {
	if err := ctx.Err(); err != nil {
		return tool.Result{Outcome: tool.OutcomeNotApplied}, err
	}
	if err := os.WriteFile(f.path, []byte("applied"), 0600); err != nil {
		return tool.Result{Outcome: tool.OutcomeNotApplied}, err
	}
	f.calls.Add(1)
	if f.after != nil {
		f.after()
	}
	return tool.Result{Output: json.RawMessage(`{"written":true}`), Outcome: f.outcome}, nil
}

type outcomeTestModel func(context.Context, model.Request) (model.Response, error)

func (f outcomeTestModel) Generate(ctx context.Context, req model.Request) (model.Response, error) {
	return f(ctx, req)
}
func (f outcomeTestModel) Stream(context.Context, model.Request, func(model.Event) error) error {
	return errors.New("本测试仅调用 Generate")
}

// 模拟工具已生效后丢失终态写入；预先保存的执行边界仍由真实仓储保存。
type lostOutcomeRepository struct {
	agent.Repository
	crash, boundary bool
}

func (r *lostOutcomeRepository) SaveStep(ctx context.Context, step agent.Step) error {
	if step.Type == "tool_call" && ((r.boundary && step.FinishedAt == nil) || (!r.boundary && step.FinishedAt != nil)) {
		return errors.New("受控结果保存失败")
	}
	return r.Repository.SaveStep(ctx, step)
}

func TestToolDoesNotExecuteIfOutcomeBoundaryCannotBeSaved(t *testing.T) {
	rt, _ := runtimeFixture(t, 1)
	instance := &outcomeTestTool{path: filepath.Join(t.TempDir(), "evidence"), outcome: tool.OutcomeApplied}
	client := outcomeTestModel(func(context.Context, model.Request) (model.Response, error) {
		return model.Response{ToolCalls: []model.ToolCall{{ID: "call-operation", Name: "operation", Arguments: json.RawMessage(`{}`)}}}, nil
	})
	id := policyTestID(t)
	svc, runs := outcomeService(t, rt, instance, client, id, func(r agent.Repository) agent.Repository {
		return &lostOutcomeRepository{Repository: r, boundary: true}
	})
	sub, err := rt.Subscribe(events.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	ex := sendOutcome(t, svc, id, "", "执行操作", nil)
	if terminal := awaitOutcome(t, rt, sub, ex.Run.ID, nil); terminal.Type != "run.failed" {
		t.Fatalf("保存失败未阻止执行: %+v", terminal)
	}
	if instance.calls.Load() != 0 {
		t.Fatal("边界保存失败后仍调用了工具")
	}
	if _, err := os.Stat(instance.path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("出现实际副作用: %v", err)
	}
	steps, err := runs.Steps(t.Context(), ex.Run.ID, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, step := range steps {
		if step.Type != "tool_call" {
			continue
		}
		found = true
		var value struct {
			Operation tool.OutcomeRecord `json:"operation"`
		}
		if json.Unmarshal(step.Output, &value) != nil || value.Operation.Outcome != tool.OutcomeNotApplied {
			t.Fatalf("未执行事实保存错误: %s", step.Output)
		}
	}
	if !found {
		t.Fatal("缺少未执行步骤")
	}
}
func (r *lostOutcomeRepository) Finish(ctx context.Context, run agent.Run) error {
	if r.crash {
		return errors.New("受控终态保存失败")
	}
	return r.Repository.Finish(ctx, run)
}

func outcomeService(t *testing.T, rt *Runtime, instance *outcomeTestTool, client model.Client, id string, wrap func(agent.Repository) agent.Repository) (*conversationservice.Service, *agentstore.Repository) {
	t.Helper()
	runs, err := agentstore.NewRepository(rt.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	conversations, err := conversationstore.NewRepository(rt.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	tools := registry.New()
	if err := tools.Register(instance); err != nil {
		t.Fatal(err)
	}
	executor, err := toolexecutor.New(tools, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var repository agent.Repository = runs
	if wrap != nil {
		repository = wrap(repository)
	}
	engine, err := agentservice.New(repository, conversations, rt.manager, rt.events.Publish, rt.logger, agentservice.Options{Tools: executor, Approvals: rt.Approvals(), Workspace: rt.Workspaces().Get, CallTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := model.Snapshot{ID: id, DisplayName: "测试模型", RemoteModelID: "test", Provider: model.ProviderSnapshot{ID: policyTestID(t), Name: "测试厂商"}}
	svc, err := conversationservice.New(conversations, func(context.Context, string) (agent.Resolved, error) {
		return agent.Resolved{Client: client, Snapshot: snapshot, ContextWindow: 1000000, Protocol: "chatcompletions"}, nil
	}, rt.manager, engine)
	if err != nil {
		t.Fatal(err)
	}
	return svc, runs
}

func sendOutcome(t *testing.T, svc *conversationservice.Service, id, conversationID, text string, accepted func(conversation.Execution)) conversation.Execution {
	t.Helper()
	var execution conversation.Execution
	err := svc.Send(t.Context(), conversation.Send{ConversationID: conversationID, ModelID: id, Stream: new(false), Content: []model.Block{{Type: "text", Text: text}}}, func(ex conversation.Execution) error {
		execution = ex
		if accepted != nil {
			accepted(ex)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return execution
}

func awaitOutcome(t *testing.T, rt *Runtime, sub *events.Subscription, runID string, onApproval func(agent.Approval)) agent.Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for {
		event, err := sub.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if event.RunID != runID {
			continue
		}
		if event.Type == "approval.requested" {
			if onApproval == nil {
				t.Fatal("出现非预期审批")
			}
			onApproval(event.Data.(agent.Approval))
		}
		if event.Type == "run.completed" || event.Type == "run.failed" || event.Type == "run.cancelled" {
			rt.manager.Wait(runID)
			return event
		}
	}
}

func TestToolFactsSurviveFailureCancellationAndRestart(t *testing.T) {
	for _, test := range []struct {
		name                                  string
		outcome                               tool.Outcome
		cancel, closeFailure, lostSave, crash bool
		want                                  tool.Outcome
	}{
		{"cancel_after_apply", tool.OutcomeApplied, true, false, false, false, tool.OutcomeApplied},
		{"close_failure", tool.OutcomeApplied, false, true, false, false, tool.OutcomeApplied},
		{"partial", tool.OutcomePartial, false, false, false, false, tool.OutcomePartial},
		{"unknown", tool.OutcomeUnknown, false, false, false, false, tool.OutcomeUnknown},
		{"save_failure", tool.OutcomeApplied, false, false, true, false, tool.OutcomeUnknown},
		{"restart_after_save_failure", tool.OutcomeApplied, false, false, true, true, tool.OutcomeUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			rt, cfg := runtimeFixture(t, 1)
			instance := &outcomeTestTool{path: filepath.Join(t.TempDir(), "evidence"), outcome: test.outcome}
			if test.closeFailure {
				instance.closeErr = errors.New("受控清理失败")
			}
			id := policyTestID(t)
			var checked atomic.Bool
			client := outcomeTestModel(func(_ context.Context, req model.Request) (model.Response, error) {
				last := req.Messages[len(req.Messages)-1]
				if last.Role == model.RoleTool {
					return model.Response{Text: "未完成回答"}, errors.New("受控后续模型失败")
				}
				if len(last.Blocks) > 0 && last.Blocks[0].Text == "继续核实" {
					found := false
					for _, message := range req.Messages {
						if strings.Contains(message.Content, "未完成回答") {
							return model.Response{}, errors.New("恢复了失败回答")
						}
						if message.Role != model.RoleTool {
							continue
						}
						var value struct {
							Operation tool.OutcomeRecord `json:"operation"`
							Result    json.RawMessage    `json:"result"`
						}
						if json.Unmarshal([]byte(message.Content), &value) != nil || value.Operation.Outcome != test.want {
							return model.Response{}, errors.New("操作事实未恢复")
						}
						if !test.lostSave && !strings.Contains(string(value.Result), `"written":true`) {
							return model.Response{}, errors.New("实际输出丢失")
						}
						found = true
					}
					if !found {
						return model.Response{}, errors.New("后续上下文缺少操作事实")
					}
					checked.Store(true)
					return model.Response{Text: "已经核实"}, nil
				}
				return model.Response{ToolCalls: []model.ToolCall{{ID: "call-operation", Name: "operation", Arguments: json.RawMessage(`{}`)}}}, nil
			})
			var wrap func(agent.Repository) agent.Repository
			if test.lostSave {
				wrap = func(r agent.Repository) agent.Repository {
					return &lostOutcomeRepository{Repository: r, crash: test.crash}
				}
			}
			svc, runs := outcomeService(t, rt, instance, client, id, wrap)
			sub, err := rt.Subscribe(events.Filter{})
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			ex := sendOutcome(t, svc, id, "", "执行操作", func(ex conversation.Execution) {
				if test.cancel {
					instance.after = func() { rt.Cancel(ex.Run.ID) }
				}
			})
			terminal := awaitOutcome(t, rt, sub, ex.Run.ID, nil)
			if terminal.Type == "run.completed" {
				t.Fatal("失败执行被标为成功")
			}
			if test.crash && (terminal.StatePersisted == nil || *terminal.StatePersisted) {
				t.Fatal("未报告终态保存失败")
			}
			if content, err := os.ReadFile(instance.path); err != nil || string(content) != "applied" {
				t.Fatalf("缺少实际操作证据: %s %v", content, err)
			}
			// 重启复用同一个临时数据目录，验证实际启动恢复调用链。
			if err := rt.Close(); err != nil {
				t.Fatal(err)
			}
			restarted, err := New(t.Context(), cfg, rt.logger)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := restarted.Close(); err != nil {
					t.Error(err)
				}
			})
			svc, runs = outcomeService(t, restarted, instance, client, id, nil)
			steps, err := runs.Steps(t.Context(), ex.Run.ID, 0, 20)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, step := range steps {
				if step.Type != "tool_call" {
					continue
				}
				found = true
				var value struct {
					Operation tool.OutcomeRecord `json:"operation"`
					Result    json.RawMessage    `json:"result"`
					Error     json.RawMessage    `json:"error"`
				}
				if json.Unmarshal(step.Output, &value) != nil || value.Operation.Outcome != test.want {
					t.Fatalf("持久化事实错误: %s", step.Output)
				}
				if !test.lostSave && !strings.Contains(string(value.Result), `"written":true`) {
					t.Fatal("持久化实际输出丢失")
				}
				if test.closeFailure && (len(value.Error) == 0 || step.Status != agent.Failed) {
					t.Fatal("清理失败未保存")
				}
			}
			if !found {
				t.Fatal("缺少操作步骤")
			}
			newSub, err := restarted.Subscribe(events.Filter{})
			if err != nil {
				t.Fatal(err)
			}
			defer newSub.Close()
			next := sendOutcome(t, svc, id, ex.Conversation.ID, "继续核实", nil)
			if terminal := awaitOutcome(t, restarted, newSub, next.Run.ID, nil); terminal.Type != "run.completed" || !checked.Load() {
				t.Fatalf("重启后的事实未传给模型: %+v", terminal)
			}
			if instance.calls.Load() != 1 {
				t.Fatal("恢复自动重试了操作")
			}
		})
	}
}

func TestUnknownOperationRetryRequiresUserDecision(t *testing.T) {
	for _, choice := range []policy.ApprovalPolicy{policy.OnRequest, policy.Never} {
		t.Run(string(choice), func(t *testing.T) {
			rt, _ := runtimeFixture(t, 1)
			setRuntimePolicy(t, rt, choice)
			instance := &outcomeTestTool{path: filepath.Join(t.TempDir(), "evidence"), outcome: tool.OutcomeUnknown}
			var calls atomic.Int32
			client := outcomeTestModel(func(_ context.Context, req model.Request) (model.Response, error) {
				n := calls.Add(1)
				if n <= 2 {
					return model.Response{ToolCalls: []model.ToolCall{{ID: []string{"first", "retry"}[n-1], Name: "operation", Arguments: json.RawMessage(`{}`)}}}, nil
				}
				return model.Response{Text: "已核实操作状态"}, nil
			})
			id := policyTestID(t)
			svc, runs := outcomeService(t, rt, instance, client, id, nil)
			sub, err := rt.Subscribe(events.Filter{})
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			ex := sendOutcome(t, svc, id, "", "执行操作", nil)
			approvals := 0
			terminal := awaitOutcome(t, rt, sub, ex.Run.ID, func(a agent.Approval) {
				approvals++
				if instance.calls.Load() != 1 || choice == policy.Never {
					t.Fatal("未知结果被自动重试")
				}
				// 仅明确的用户决定授权本次固定操作。
				instance.outcome = tool.OutcomeApplied
				if _, err := rt.Approvals().Decide(t.Context(), a.ID, agent.ApprovalApproved, nil); err != nil {
					t.Fatal(err)
				}
			})
			if terminal.Type != "run.completed" {
				t.Fatalf("执行异常: %+v", terminal)
			}
			want := int32(1)
			if choice == policy.OnRequest {
				want = 2
				if approvals != 1 {
					t.Fatal("缺少重试审批")
				}
			}
			if instance.calls.Load() != want {
				t.Fatalf("实际执行次数: %d", instance.calls.Load())
			}
			steps, err := runs.Steps(t.Context(), ex.Run.ID, 0, 20)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, step := range steps {
				if step.ToolCallID != nil && *step.ToolCallID == "retry" {
					found = true
					if !strings.Contains(string(step.Input), "previous_result_requires_confirmation") {
						t.Fatal("缺少重试判定审计")
					}
				}
			}
			if !found {
				t.Fatal("缺少重试步骤")
			}
		})
	}
}

func TestUnknownOperationRetryAfterRestartRequiresApproval(t *testing.T) {
	rt, cfg := runtimeFixture(t, 1)
	instance := &outcomeTestTool{path: filepath.Join(t.TempDir(), "evidence"), outcome: tool.OutcomeUnknown}
	client := outcomeTestModel(func(_ context.Context, req model.Request) (model.Response, error) {
		if req.Messages[len(req.Messages)-1].Role == model.RoleTool {
			return model.Response{Text: "等待核实"}, nil
		}
		return model.Response{ToolCalls: []model.ToolCall{{ID: "call-operation", Name: "operation", Arguments: json.RawMessage(`{}`)}}}, nil
	})
	id := policyTestID(t)
	svc, _ := outcomeService(t, rt, instance, client, id, nil)
	sub, err := rt.Subscribe(events.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	ex := sendOutcome(t, svc, id, "", "执行操作", nil)
	if terminal := awaitOutcome(t, rt, sub, ex.Run.ID, nil); terminal.Type != "run.completed" {
		t.Fatalf("执行异常: %+v", terminal)
	}
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(t.Context(), cfg, rt.logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := restarted.Close(); err != nil {
			t.Error(err)
		}
	})
	svc, _ = outcomeService(t, restarted, instance, client, id, nil)
	sub, err = restarted.Subscribe(events.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	next := sendOutcome(t, svc, id, ex.Conversation.ID, "核实后重试", nil)
	approvals := 0
	terminal := awaitOutcome(t, restarted, sub, next.Run.ID, func(a agent.Approval) {
		approvals++
		if instance.calls.Load() != 1 {
			t.Fatal("重启后自动重试了未知操作")
		}
		instance.outcome = tool.OutcomeApplied
		if _, err := restarted.Approvals().Decide(t.Context(), a.ID, agent.ApprovalApproved, nil); err != nil {
			t.Fatal(err)
		}
	})
	if terminal.Type != "run.completed" || approvals != 1 || instance.calls.Load() != 2 {
		t.Fatalf("重试审批未贯通: %+v approvals=%d calls=%d", terminal, approvals, instance.calls.Load())
	}
}
