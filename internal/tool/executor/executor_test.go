package executor_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/executor"
	"github.com/xiaoyu98628/callum/internal/tool/registry"
)

type mutableTool struct {
	definition tool.Definition
	executed   bool
	target     *tool.Target
	prepareErr error
	closeErr   error
	closes     int
	result     *tool.Result
	executeErr error
	onExecute  func()
}

func (t *mutableTool) Definition() tool.Definition { return t.definition }
func (t *mutableTool) Validate(json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}
func (t *mutableTool) Prepare(context.Context, tool.Scope, json.RawMessage) (tool.Invocation, error) {
	return t, t.prepareErr
}
func (t *mutableTool) Target() *tool.Target { return t.target }
func (t *mutableTool) Close() error         { t.closes++; return t.closeErr }
func (t *mutableTool) Execute(context.Context) (tool.Result, error) {
	t.executed = true
	if t.onExecute != nil {
		t.onExecute()
	}
	if t.result != nil {
		return *t.result, t.executeErr
	}
	return tool.Result{Output: json.RawMessage(`{}`), Outcome: tool.OutcomeNone}, nil
}

func TestExecutionPreservesEffectsAndOutputOnFailure(t *testing.T) {
	for _, test := range []struct {
		name         string
		result       tool.Result
		err, wantErr error
		cancel       bool
		wantOutcome  tool.Outcome
		wantOutput   bool
	}{
		{"cancel_after_apply", tool.Result{Output: json.RawMessage(`{"written":true}`), Outcome: tool.OutcomeApplied}, nil, context.Canceled, true, tool.OutcomeApplied, true},
		{"error_after_apply", tool.Result{Output: json.RawMessage(`{"written":true}`), Outcome: tool.OutcomeApplied}, tool.ErrUnavailable, tool.ErrUnavailable, false, tool.OutcomeApplied, true},
		{"partial", tool.Result{Output: json.RawMessage(`{"written":1}`), Outcome: tool.OutcomePartial}, nil, tool.ErrPartialOutcome, false, tool.OutcomePartial, true},
		{"unknown", tool.Result{Outcome: tool.OutcomeUnknown}, nil, tool.ErrUnknownOutcome, false, tool.OutcomeUnknown, false},
		{"missing_outcome", tool.Result{Output: json.RawMessage(`{}`)}, nil, tool.ErrInvalid, false, tool.OutcomeUnknown, true},
		{"oversized_output", tool.Result{Output: json.RawMessage(`"` + strings.Repeat("x", tool.MaxResultBytes) + `"`), Outcome: tool.OutcomeApplied}, nil, tool.ErrLimit, false, tool.OutcomeApplied, false},
		{"invalid_output", tool.Result{Output: json.RawMessage(`invalid`), Outcome: tool.OutcomeApplied}, nil, tool.ErrInvalid, false, tool.OutcomeApplied, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			instance := &mutableTool{definition: tool.Definition{ToolDefinition: model.ToolDefinition{Name: "operation", Description: "操作结果验证", Parameters: json.RawMessage(`{}`)}, Source: "test", Version: "1", Effects: []tool.Effect{tool.EffectWrite}}, result: &test.result, executeErr: test.err}
			if test.cancel {
				instance.onExecute = cancel
			}
			r := registry.New()
			if err := r.Register(instance); err != nil {
				t.Fatal(err)
			}
			e, err := executor.New(r, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			p, err := e.Prepare(ctx, tool.Scope{}, "operation", json.RawMessage(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := p.Close(); err != nil {
					t.Error(err)
				}
			}()
			result, err := e.Execute(ctx, p, p.Hash())
			if !instance.executed || !errors.Is(err, test.wantErr) || result.Outcome != test.wantOutcome || (result.Output != nil) != test.wantOutput {
				t.Fatalf("操作事实或错误丢失: outcome=%s output=%s err=%v", result.Outcome, result.Output, err)
			}
			if test.wantOutput && string(result.Output) != string(test.result.Output) {
				t.Fatal("实际输出被覆盖")
			}
		})
	}
}

func TestOperationEffectsAreFrozenBeforeApproval(t *testing.T) {
	instance := &mutableTool{definition: tool.Definition{
		ToolDefinition: model.ToolDefinition{Name: "operation", Description: "测试工具", Parameters: json.RawMessage(`{"type":"object"}`)},
		Source:         "test", Version: "1", Effects: []tool.Effect{tool.EffectDestructive},
	}}
	r := registry.New()
	if err := r.Register(instance); err != nil {
		t.Fatal(err)
	}
	// 返回值不能借用注册工具的可变类别切片。
	listed := r.Definitions()
	listed[0].Effects[0] = tool.EffectRead
	if instance.definition.Effects[0] != tool.EffectDestructive {
		t.Fatal("查询结果修改了注册工具权限")
	}
	e, err := executor.New(r, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.Prepare(t.Context(), tool.Scope{ID: "test-workspace", RootPath: t.TempDir()}, "operation", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := p.Close(); err != nil {
			t.Error(err)
		}
	}()
	definition := p.Definition()
	definition.Effects[0] = tool.EffectRead
	if p.Definition().Effects[0] != tool.EffectDestructive {
		t.Fatal("调用方修改了固定操作的类别")
	}
	// 注册工具在等待审批时改变类别，不能覆盖快照，也不能继续执行。
	instance.definition.Effects[0] = tool.EffectRead
	if p.Definition().Effects[0] != tool.EffectDestructive {
		t.Fatal("工具变化修改了已经固定的操作")
	}
	if _, err := e.Execute(t.Context(), p, p.Hash()); !errors.Is(err, tool.ErrDenied) || instance.executed {
		t.Fatalf("定义变化后仍执行工具: %v", err)
	}
}

func TestRegistryRejectsInvalidEffect(t *testing.T) {
	r := registry.New()
	instance := &mutableTool{definition: tool.Definition{
		ToolDefinition: model.ToolDefinition{Name: "operation", Description: "测试工具", Parameters: json.RawMessage(`{}`)},
		Source:         "test", Version: "1", Effects: []tool.Effect{"invalid"},
	}}
	if err := r.Register(instance); !errors.Is(err, tool.ErrInvalid) {
		t.Fatalf("注册了无效类别: %v", err)
	}
	if len(r.Definitions()) != 0 {
		t.Fatal("失败的注册污染了工具列表")
	}
}

func TestNonFileToolDoesNotRequireWorkspace(t *testing.T) {
	instance := &mutableTool{definition: tool.Definition{Name: "operation", Description: "测试工具", Parameters: json.RawMessage(`{"type":"object"}`), Source: "test", Version: "1", Effects: []tool.Effect{tool.EffectRead}}}
	r := registry.New()
	if err := r.Register(instance); err != nil {
		t.Fatal(err)
	}
	e, err := executor.New(r, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.Prepare(t.Context(), tool.Scope{}, "operation", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := p.Close(); err != nil {
			t.Error(err)
		}
	}()
	if _, err := e.Execute(t.Context(), p, p.Hash()); err != nil || !instance.executed {
		t.Fatalf("非文件工具被工作区限制: %v", err)
	}
}

func TestTargetSnapshotAndPreparationCleanup(t *testing.T) {
	for _, failure := range []bool{false, true} {
		instance := &mutableTool{definition: tool.Definition{Name: "operation", Description: "测试工具", Parameters: json.RawMessage(`{"type":"object"}`), Source: "test", Version: "1", Effects: []tool.Effect{tool.EffectRead}}, target: &tool.Target{Path: "/fixed-target", Kind: "file"}}
		prepareErr, closeErr := errors.New("prepare failed"), errors.New("close failed")
		if failure {
			instance.prepareErr, instance.closeErr = prepareErr, closeErr
		}
		r := registry.New()
		if err := r.Register(instance); err != nil {
			t.Fatal(err)
		}
		e, err := executor.New(r, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		p, err := e.Prepare(t.Context(), tool.Scope{}, "operation", json.RawMessage(`{}`))
		if failure {
			if p != nil || !errors.Is(err, prepareErr) || !errors.Is(err, closeErr) || instance.closes != 1 {
				t.Fatalf("准备失败未清理资源或丢失原始错误: %v, closes=%d", err, instance.closes)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		instance.target.Path = "/changed-target"
		if _, err := e.Execute(t.Context(), p, p.Hash()); !errors.Is(err, tool.ErrDenied) || instance.executed {
			t.Fatalf("目标被替换后仍执行: %v", err)
		}
		if err := p.Close(); err != nil {
			t.Fatal(err)
		}
		if err := p.Close(); err != nil || instance.closes != 1 {
			t.Fatalf("重复关闭未保持安全: %v, closes=%d", err, instance.closes)
		}
	}
}

type plannedTool struct {
	mutableTool
	plan tool.Plan
}

func (t *plannedTool) Prepare(context.Context, tool.Scope, json.RawMessage) (tool.Invocation, error) {
	return t, nil
}
func (t *plannedTool) Plan() tool.Plan { return t.plan }

func TestMultiTargetPlanCannotChangeAfterApproval(t *testing.T) {
	for _, change := range []string{"target", "effect", "none"} {
		t.Run(change, func(t *testing.T) {
			dir := t.TempDir()
			a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
			instance := &plannedTool{definition: tool.Definition{Name: "operation", Description: "多目标审批", Parameters: json.RawMessage(`{}`), Source: "test", Version: "1", Effects: []tool.Effect{tool.EffectRead}}, plan: tool.Plan{Targets: []tool.Target{{RequestedPath: a, Path: a, Kind: "file", WithinWorkspace: true}, {RequestedPath: b, Path: b, Kind: "file", WithinWorkspace: false}}, AdditionalEffects: []tool.Effect{tool.EffectDestructive}}}
			r := registry.New()
			if err := r.Register(instance); err != nil {
				t.Fatal(err)
			}
			e, err := executor.New(r, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			p, err := e.Prepare(t.Context(), tool.Scope{}, "operation", json.RawMessage(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := p.Close(); err != nil {
					t.Error(err)
				}
			}()
			if !p.OutsideWorkspace() || !slices.Contains(p.Definition().Effects, tool.EffectDestructive) {
				t.Fatal("多目标或额外影响丢失")
			}
			before := string(p.Request())
			switch change {
			case "target":
				instance.plan.Targets[1].WithinWorkspace = true
			case "effect":
				instance.plan.AdditionalEffects[0] = tool.EffectRead
			}
			if string(p.Request()) != before {
				t.Fatal("共享切片改变了审批记录")
			}
			_, err = e.Execute(t.Context(), p, p.Hash())
			if change == "none" {
				if err != nil || !instance.executed {
					t.Fatalf("未变化计划无法执行: %v", err)
				}
			} else if !errors.Is(err, tool.ErrDenied) || instance.executed {
				t.Fatalf("计划变化后仍执行: %v", err)
			}
		})
	}
}

func TestPlanRejectsInvalidEffectsAndPreservesUnclassifiedRisk(t *testing.T) {
	for _, effect := range []tool.Effect{tool.EffectRead, "invalid"} {
		t.Run(string(effect), func(t *testing.T) {
			instance := &plannedTool{definition: tool.Definition{Name: "operation", Description: "额外影响校验", Parameters: json.RawMessage(`{}`), Source: "test", Version: "1"}, plan: tool.Plan{AdditionalEffects: []tool.Effect{effect}}}
			r := registry.New()
			if err := r.Register(instance); err != nil {
				t.Fatal(err)
			}
			e, err := executor.New(r, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			p, err := e.Prepare(t.Context(), tool.Scope{}, "operation", json.RawMessage(`{}`))
			if effect == "invalid" {
				if !errors.Is(err, tool.ErrInvalid) || instance.closes != 1 {
					t.Fatalf("无效影响未被拒绝或资源泄漏: %v closes=%d", err, instance.closes)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			if !slices.Contains(p.Definition().Effects, tool.EffectUnknown) {
				t.Fatal("额外读取声明抵消了未知影响")
			}
		})
	}
}

func TestMissingOutcomeIncludesPlannedWriteEffects(t *testing.T) {
	instance := &plannedTool{definition: tool.Definition{Name: "operation", Description: "动态修改影响", Parameters: json.RawMessage(`{}`), Source: "test", Version: "1", Effects: []tool.Effect{tool.EffectRead}}, result: &tool.Result{Output: json.RawMessage(`{}`)}, plan: tool.Plan{AdditionalEffects: []tool.Effect{tool.EffectWrite}}}
	r := registry.New()
	if err := r.Register(instance); err != nil {
		t.Fatal(err)
	}
	e, err := executor.New(r, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.Prepare(t.Context(), tool.Scope{}, "operation", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := p.Close(); err != nil {
			t.Error(err)
		}
	}()
	result, err := e.Execute(t.Context(), p, p.Hash())
	if !errors.Is(err, tool.ErrInvalid) || result.Outcome != tool.OutcomeUnknown {
		t.Fatalf("忽略额外写入影响而推断为无副作用: %+v %v", result, err)
	}
}
