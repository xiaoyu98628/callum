// Package runtime 提供技能所需的本地依赖路径，不安装依赖或执行进程。
package runtime

import (
	"context"
	"encoding/json"

	"github.com/xiaoyu98628/callum/internal/runtimeassets"
	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/internal/arguments"
)

type dependenciesTool struct{ bundle *runtimeassets.Bundle }

func New(bundle *runtimeassets.Bundle) tool.Tool { return dependenciesTool{bundle} }

func (dependenciesTool) Definition() tool.Definition {
	return tool.Definition{Name: "load_workspace_dependencies", Description: "返回 Callum 随附 工作区技能的实际 Python、Node、node_modules、原生工具目录和本轮私有工作目录。仅查询已校验的本地资源，不联网、不安装依赖。工作区技能应使用这些绝对路径。", Parameters: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`), Source: "builtin", Version: "1", Effects: []tool.Effect{tool.EffectRead}}
}

func (dependenciesTool) Validate(raw json.RawMessage) (json.RawMessage, error) {
	var value struct{}
	if _, err := arguments.Decode(raw, &value); err != nil {
		return nil, err
	}
	return json.RawMessage(`{}`), nil
}

func (t dependenciesTool) Prepare(ctx context.Context, scope tool.Scope, raw json.RawMessage) (tool.Invocation, error) {
	return t.PrepareWithPermissions(ctx, scope, raw, tool.ExecutionPermissions{})
}

func (t dependenciesTool) PrepareWithPermissions(ctx context.Context, _ tool.Scope, raw json.RawMessage, permissions tool.ExecutionPermissions) (tool.Invocation, error) {
	if _, err := t.Validate(raw); err != nil {
		return nil, err
	}
	if !t.bundle.HasWorkspaceDependencies() {
		return nil, tool.ErrUnavailable
	}
	if err := t.bundle.Check(ctx); err != nil {
		return nil, tool.ErrUnavailable
	}
	paths := t.bundle.WorkspacePaths()
	paths["work_directory"] = permissions.WorkDirectory
	output, err := json.Marshal(paths)
	return dependencyInvocation{output}, err
}

type dependencyInvocation struct{ output json.RawMessage }

func (dependencyInvocation) Target() *tool.Target { return nil }
func (dependencyInvocation) Close() error         { return nil }
func (p dependencyInvocation) Execute(ctx context.Context) (tool.Result, error) {
	return tool.Result{Output: p.output, Outcome: tool.OutcomeNone}, ctx.Err()
}
