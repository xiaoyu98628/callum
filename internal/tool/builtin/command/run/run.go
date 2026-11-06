// Package run 提供非交互命令执行，macOS 可采用服务端固定的系统沙箱边界。
package run

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/xiaoyu98628/callum/internal/tool"
)

type commandTool struct{ environment []string }

// New 从装配层传入的环境中固定非敏感基础变量；不读取或修改全局环境。
func New(environment []string) tool.Tool {
	return &commandTool{environment: baseEnvironment(environment)}
}

func (*commandTool) Definition() tool.Definition {
	return tool.Definition{Name: "run_command", Description: "执行非交互命令并返回退出码及有界 stdout/stderr。用 exit_code 判断命令成功，非零表示命令失败。command 为可执行文件和参数组成的数组，不自动套 Shell。cwd 的相对路径或省略值以工作区为基准；未绑定工作区时必须提供绝对 cwd。命令默认在服务端配置的系统沙箱中执行，禁止网络且限制写入范围。沙箱拒绝操作时不要自动重试；确需更大权限时，使用 sandbox_permissions=require_escalated 并填写 justification 请求本次沙箱外执行，必须等待用户审批。不支持交互输入、PTY 或持久后台会话。", Parameters: json.RawMessage(`{"type":"object","properties":{"command":{"type":"array","items":{"type":"string"},"minItems":1,"maxItems":256},"cwd":{"type":"string"},"sandbox_permissions":{"type":"string","enum":["use_default","require_escalated"]},"justification":{"type":"string"}},"required":["command"],"additionalProperties":false}`), Source: "builtin", Version: "2", Effects: []tool.Effect{tool.EffectUnknown}}
}

func (*commandTool) Validate(raw json.RawMessage) (json.RawMessage, error) {
	a, err := decodeArguments(raw)
	if err != nil {
		return nil, err
	}
	return json.Marshal(a)
}

func (t *commandTool) Prepare(ctx context.Context, scope tool.Scope, raw json.RawMessage) (tool.Invocation, error) {
	return t.PrepareWithPermissions(ctx, scope, raw, tool.ExecutionPermissions{})
}

func (t *commandTool) PrepareWithPermissions(ctx context.Context, scope tool.Scope, raw json.RawMessage, permissions tool.ExecutionPermissions) (tool.Invocation, error) {
	a, err := decodeArguments(raw)
	if err != nil {
		return nil, err
	}
	return prepare(ctx, scope, a, slices.Clone(t.environment), permissions)
}
