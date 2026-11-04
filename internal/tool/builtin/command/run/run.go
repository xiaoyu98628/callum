// Package run 提供需审批的非交互命令执行，不提供文件系统或网络沙箱。
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
	return tool.Definition{Name: "run_command", Description: "执行非交互命令并返回退出码及有界 stdout/stderr。用 exit_code 判断命令成功，非零表示命令失败。command 为可执行文件和参数组成的数组，不自动套 Shell。cwd 的相对路径或省略值以工作区为基准；未绑定工作区时必须提供绝对 cwd。命令按当前用户权限执行，不受文件系统或网络沙箱限制，必须遵循审批决定。不支持交互输入、PTY 或持久后台会话。", Parameters: json.RawMessage(`{"type":"object","properties":{"command":{"type":"array","items":{"type":"string"},"minItems":1,"maxItems":256},"cwd":{"type":"string"}},"required":["command"],"additionalProperties":false}`), Source: "builtin", Version: "1", Effects: []tool.Effect{tool.EffectUnknown}}
}

func (*commandTool) Validate(raw json.RawMessage) (json.RawMessage, error) {
	a, err := decodeArguments(raw)
	if err != nil {
		return nil, err
	}
	return json.Marshal(a)
}

func (t *commandTool) Prepare(ctx context.Context, scope tool.Scope, raw json.RawMessage) (tool.Invocation, error) {
	a, err := decodeArguments(raw)
	if err != nil {
		return nil, err
	}
	return prepare(ctx, scope, a, slices.Clone(t.environment))
}
