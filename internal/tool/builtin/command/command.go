// Package command 汇总命令工具，由具体工具负责参数、审批详情与进程生命周期。
package command

import (
	"context"
	"github.com/xiaoyu98628/callum/internal/runtimeassets"
	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/command/run"
)

// Tools 返回独立的工具实例，将装配层环境交给具体工具筛选并固定。
func Tools(environment []string, protection tool.ProtectedData) []tool.Tool {
	return []tool.Tool{run.New(environment, protection)}
}

// ToolsWithRuntime 将只读解释器资源交给现有命令工具，资源生命周期由应用持有。
func ToolsWithRuntime(environment []string, protection tool.ProtectedData, bundle *runtimeassets.Bundle) []tool.Tool {
	return []tool.Tool{run.NewWithRuntime(environment, protection, bundle)}
}

// RecoverSandbox 在应用取得数据目录实例锁后、迁移和执行恢复之前回收中断的沙箱。
func RecoverSandbox(ctx context.Context, dataDirectory string) error {
	return run.RecoverSandbox(ctx, dataDirectory)
}
