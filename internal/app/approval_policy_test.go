package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/events"
	agentservice "github.com/xiaoyu98628/callum/internal/agent/service"
	"github.com/xiaoyu98628/callum/internal/conversation"
	conversationservice "github.com/xiaoyu98628/callum/internal/conversation/service"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/settings"
	agentstore "github.com/xiaoyu98628/callum/internal/storage/sqlite/agent"
	conversationstore "github.com/xiaoyu98628/callum/internal/storage/sqlite/conversation"
	"github.com/xiaoyu98628/callum/internal/tool"
	toolexecutor "github.com/xiaoyu98628/callum/internal/tool/executor"
	"github.com/xiaoyu98628/callum/internal/tool/policy"
	"github.com/xiaoyu98628/callum/internal/tool/registry"
)

// policyTestTool 只记录是否真正执行，用可信类别模拟尚未接入的操作，不执行外部操作。
type policyTestTool struct {
	effects []tool.Effect
	calls   atomic.Int32
}

func (f *policyTestTool) Definition() tool.Definition {
	return tool.Definition{Name: "test_operation", Description: "审批判定测试", Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`), Source: "test", Version: "1", Effects: f.effects}
}
func (f *policyTestTool) Validate(args json.RawMessage) (json.RawMessage, error) {
	if string(args) != "{}" {
		return nil, tool.ErrInvalid
	}
	return json.RawMessage(`{}`), nil
}
func (f *policyTestTool) Prepare(context.Context, tool.Scope, json.RawMessage) (tool.Invocation, error) {
	return f, nil
}
func (f *policyTestTool) Target() *tool.Target { return nil }
func (f *policyTestTool) Close() error         { return nil }
func (f *policyTestTool) Execute(ctx context.Context) (tool.Result, error) {
	if err := ctx.Err(); err != nil {
		return tool.Result{Outcome: tool.OutcomeNotApplied}, err
	}
	f.calls.Add(1)
	outcome := tool.OutcomeApplied
	if tool.UnconfirmedOutcome(f.effects) == tool.OutcomeNone {
		outcome = tool.OutcomeNone
	}
	return tool.Result{Output: json.RawMessage(`{"executed":true}`), Outcome: outcome}, nil
}

type policyTestModel struct {
	args   json.RawMessage
	denied bool
}

func (f policyTestModel) Generate(ctx context.Context, req model.Request) (model.Response, error) {
	if err := ctx.Err(); err != nil {
		return model.Response{}, err
	}
	last := req.Messages[len(req.Messages)-1]
	if last.Role == model.RoleTool {
		if bytes.Contains([]byte(last.Content), []byte(`"error"`)) != f.denied {
			return model.Response{}, errors.New("工具权限结果与预期不符")
		}
		return model.Response{Text: "完成"}, nil
	}
	return model.Response{ToolCalls: []model.ToolCall{{ID: "call-operation", Name: "test_operation", Arguments: f.args}}}, nil
}
func (f policyTestModel) Stream(context.Context, model.Request, func(model.Event) error) error {
	return errors.New("本测试仅调用 Generate")
}

func TestToolApprovalPoliciesBeforeExecution(t *testing.T) {
	for _, test := range []struct {
		name       string
		policy     policy.ApprovalPolicy
		effects    []tool.Effect
		args       string
		action     policy.Action
		wantReason string
	}{
		{"on_request_read", policy.OnRequest, []tool.Effect{tool.EffectRead}, "{}", policy.Allow, "workspace_read_write_policy"},
		{"on_request_write", policy.OnRequest, []tool.Effect{tool.EffectWrite}, "{}", policy.Allow, "workspace_read_write_policy"},
		{"on_request_delete", policy.OnRequest, []tool.Effect{tool.EffectDestructive}, "{}", policy.Ask, "destructive_operation"},
		{"on_request_external", policy.OnRequest, []tool.Effect{tool.EffectExternal}, "{}", policy.Ask, "external_operation"},
		{"on_request_unclassified", policy.OnRequest, nil, "{}", policy.Ask, "unclassified_operation"},
		{"always_read", policy.Always, []tool.Effect{tool.EffectRead}, "{}", policy.Ask, "policy_always"},
		{"never_read", policy.Never, []tool.Effect{tool.EffectRead}, "{}", policy.Allow, "workspace_read_write_policy"},
		{"never_write", policy.Never, []tool.Effect{tool.EffectWrite}, "{}", policy.Allow, "workspace_read_write_policy"},
		{"never_read_delete", policy.Never, []tool.Effect{tool.EffectRead, tool.EffectDestructive}, "{}", policy.Deny, "approval_required_but_disabled"},
		{"never_external", policy.Never, []tool.Effect{tool.EffectExternal}, "{}", policy.Deny, "approval_required_but_disabled"},
		{"never_unclassified", policy.Never, nil, "{}", policy.Deny, "approval_required_but_disabled"},
		{"always_invalid_args", policy.Always, []tool.Effect{tool.EffectRead}, `{"effects":["read"]}`, policy.Deny, "operation_validation_failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime, cfg := runtimeFixture(t, 1)
			setRuntimePolicy(t, runtime, test.policy)
			instance := &policyTestTool{effects: test.effects}
			tools := registry.New()
			if err := tools.Register(instance); err != nil {
				t.Fatal(err)
			}
			executor, err := toolexecutor.New(tools, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			runs, err := agentstore.NewRepository(runtime.db.DB)
			if err != nil {
				t.Fatal(err)
			}
			conversations, err := conversationstore.NewRepository(runtime.db.DB)
			if err != nil {
				t.Fatal(err)
			}
			engine, err := agentservice.New(runs, conversations, runtime.manager, runtime.events.Publish, runtime.logger,
				agentservice.Options{Tools: executor, Approvals: runtime.Approvals(), Workspace: runtime.Workspaces().Get,
					CallTimeout: cfg.CallTimeout})
			if err != nil {
				t.Fatal(err)
			}
			snapshot := model.Snapshot{ID: policyTestID(t), DisplayName: "测试模型", RemoteModelID: "test", Provider: model.ProviderSnapshot{ID: policyTestID(t), Name: "测试厂商"}}
			service, err := conversationservice.New(conversations, func(context.Context, string) (agent.Resolved, error) {
				return agent.Resolved{Client: policyTestModel{args: json.RawMessage(test.args), denied: test.action == policy.Deny}, Snapshot: snapshot, Protocol: "chatcompletions"}, nil
			}, runtime.manager, engine)
			if err != nil {
				t.Fatal(err)
			}
			workspace, err := runtime.Workspaces().Create(t.Context(), "测试工作区", t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			sub, err := runtime.Subscribe(events.Filter{})
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			var accepted conversation.Execution
			err = service.Send(t.Context(), conversation.Send{ModelID: snapshot.ID, WorkspaceID: &workspace.ID, Stream: new(false), Content: []model.Block{{Type: "text", Text: "执行操作"}}}, func(ex conversation.Execution) error {
				accepted = ex
				if test.name == "always_read" {
					// 调用方修改回调中的快照，不能改变服务内部已经受理的权限。
					ex.Run.SettingsSnapshot.Revision = 0
					ex.Run.SettingsSnapshot.ApprovalPolicy = policy.Never
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			approvals := 0
		loop:
			for {
				event, err := sub.Next(ctx)
				if err != nil {
					t.Fatal(err)
				}
				switch event.Type {
				case "approval.requested":
					if test.action != policy.Ask || instance.calls.Load() != 0 {
						t.Fatal("出现不应有的审批，或审批前已执行")
					}
					approvals++
					value := event.Data.(agent.Approval)
					if _, err := runtime.Approvals().Decide(t.Context(), value.ID, agent.ApprovalApproved, nil); err != nil {
						t.Fatal(err)
					}
				case "run.completed":
					break loop
				case "run.failed", "run.cancelled":
					t.Fatalf("执行未完成: %+v", event)
				}
			}
			runtime.manager.Wait(accepted.Run.ID)
			wantCalls := int32(1)
			if test.action == policy.Deny {
				wantCalls = 0
			}
			if instance.calls.Load() != wantCalls {
				t.Fatalf("实际工具执行次数: %d，期望 %d", instance.calls.Load(), wantCalls)
			}
			rows, err := runtime.Approvals().List(t.Context(), agent.ApprovalFilter{RunID: accepted.Run.ID})
			if err != nil || len(rows) != approvals || (test.action == policy.Ask && approvals != 1) {
				t.Fatalf("审批记录数量不符: %+v %v", rows, err)
			}
			steps, err := engine.Steps(t.Context(), accepted.Run.ID, 0, 20)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, step := range steps {
				if step.Type != "tool_call" {
					continue
				}
				found = true
				var input struct {
					Policy        policy.ApprovalPolicy `json:"approval_policy"`
					Reviewer      policy.Reviewer       `json:"approvals_reviewer"`
					Authorization policy.Decision       `json:"authorization"`
				}
				if err := json.Unmarshal(step.Input, &input); err != nil {
					t.Fatal(err)
				}
				if input.Policy != test.policy || input.Reviewer != policy.User || input.Authorization.Action != test.action || input.Authorization.Reason != test.wantReason {
					t.Fatalf("审批判定审计不完整: %s", step.Input)
				}
				if test.action == policy.Deny && (step.Status != agent.Failed || step.StartedAt != nil) {
					t.Fatal("被拒绝的操作出现执行标记")
				}
			}
			if !found {
				t.Fatal("缺少工具步骤")
			}
		})
	}
}

func policyTestID(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func setRuntimePolicy(t *testing.T, runtime *Runtime, choice policy.ApprovalPolicy) settings.Record {
	t.Helper()
	current, err := runtime.Settings().Get(t.Context(), settings.ExecutionKey)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(settings.Execution{ApprovalPolicy: choice, ApprovalsReviewer: policy.User})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := runtime.Settings().Update(t.Context(), settings.ExecutionKey, current.Revision, raw)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func TestInvalidApprovalSettingsKeepRevision(t *testing.T) {
	runtime, _ := runtimeFixture(t, 1)
	current, err := runtime.Settings().Get(t.Context(), settings.ExecutionKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{}`, `{"approval_policy":"never"}`, `{"approval_policy":"invalid","approvals_reviewer":"user"}`,
		`{"approval_policy":"on-request","approvals_reviewer":"auto_review"}`,
		`{"approval_policy":"on-request","approvals_reviewer":"user","api_key":"forbidden-test-value"}`,
	} {
		if _, err := runtime.Settings().Update(t.Context(), settings.ExecutionKey, current.Revision, json.RawMessage(raw)); !errors.Is(err, settings.ErrInvalid) {
			t.Fatalf("未拒绝非法设置: %v", err)
		}
	}
	saved, err := runtime.Settings().Get(t.Context(), settings.ExecutionKey)
	if err != nil || saved.Revision != current.Revision || !bytes.Equal(saved.Value, current.Value) {
		t.Fatalf("非法请求修改了持久化设置: %+v %v", saved, err)
	}
}
