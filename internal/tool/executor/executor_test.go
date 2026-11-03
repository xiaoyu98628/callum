package executor_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
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
}

func (t *mutableTool) Definition() tool.Definition { return t.definition }
func (t *mutableTool) Validate(json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}
func (t *mutableTool) Execute(context.Context, *os.Root, json.RawMessage) (json.RawMessage, error) {
	t.executed = true
	return json.RawMessage(`{}`), nil
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
