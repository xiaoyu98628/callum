// Package run 提供非交互命令执行，macOS 和 Windows 可采用服务端固定的系统沙箱边界。
package run

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/xiaoyu98628/callum/internal/runtimeassets"
	"github.com/xiaoyu98628/callum/internal/tool"
)

type commandTool struct {
	environment []string
	protection  tool.ProtectedData
	runtime     *runtimeassets.Bundle
}

// New 从装配层传入的环境中固定非敏感基础变量；不读取或修改全局环境。
func New(environment []string, protection tool.ProtectedData) tool.Tool {
	return NewWithRuntime(environment, protection, nil)
}

// NewWithRuntime 借用已校验资源，只设置命令子进程环境，不改变服务进程环境。
func NewWithRuntime(environment []string, protection tool.ProtectedData, bundle *runtimeassets.Bundle) tool.Tool {
	filtered := baseEnvironment(environment)
	protection.ReadOnlyDirectories = slices.Clone(protection.ReadOnlyDirectories)
	if bundle != nil {
		paths := append(bundle.BinDirectories(), filepath.SplitList(environmentValue(filtered, "PATH"))...)
		filtered = slices.DeleteFunc(filtered, func(entry string) bool { return strings.HasPrefix(entry, "PATH=") })
		filtered = append(filtered, "PATH="+strings.Join(paths, string(filepath.ListSeparator)), "PYTHONDONTWRITEBYTECODE=1", "PYTHONNOUSERSITE=1", "PYTHONUTF8=1")
		if !slices.Contains(protection.ReadOnlyDirectories, bundle.Directory()) {
			protection.ReadOnlyDirectories = append(protection.ReadOnlyDirectories, bundle.Directory())
		}
	}
	return &commandTool{environment: filtered, protection: protection, runtime: bundle}
}

func (t *commandTool) Definition() tool.Definition {
	definition := tool.Definition{Name: "run_command", Description: "执行非交互命令并返回退出码及有界 stdout/stderr。用 exit_code 判断命令成功，非零表示命令失败。command 为可执行文件和参数组成的数组，不自动套 Shell。cwd 的相对路径或省略值以工作区为基准；未绑定工作区时必须提供绝对 cwd。macOS 命令默认在只读沙箱执行，禁止网络，仅私有临时目录可写。需要在项目内运行写入命令时，用 sandbox_permissions=workspace_write 和 justification 请求本次沙箱内项目写入，须等待审批。确需沙箱外权限时使用 require_escalated 并说明理由。沙箱拒绝后不自动重试或提权。不支持交互输入、PTY 或持久后台会话。", Parameters: json.RawMessage(`{"type":"object","properties":{"command":{"type":"array","items":{"type":"string"},"minItems":1,"maxItems":256},"cwd":{"type":"string"},"sandbox_permissions":{"type":"string","enum":["use_default","workspace_write","require_escalated"]},"justification":{"type":"string"}},"required":["command"],"additionalProperties":false}`), Source: "builtin", Version: "3", Effects: []tool.Effect{tool.EffectUnknown}}
	if runtime.GOOS == "windows" {
		definition.Version = "4"
		definition.Description = "执行 Windows 非交互命令，command 为 exe/com 及参数数组，不自动套 Shell。脚本需显式指定 powershell.exe 或 cmd.exe。用 exit_code 判断成功。cwd 相对路径或省略值以工作区为基准，未绑定时必须提供绝对 cwd。默认使用 AppContainer 原生只读沙箱，网络关闭，本次私有临时目录和隔离配置存储可写。读取范围为项目、cwd、程序目录及 Windows 已授予访问的系统资源。读取其他位置可使用文件工具。需要命令修改项目时提交 sandbox_permissions=workspace_write 和非空 justification，等待本次审批。需要沙箱外权限时使用 require_escalated 并说明理由。沙箱建立或权限复核失败明确失败，拒绝后不自动重试或提权。不支持交互输入、PTY 或持久后台会话。"
	}
	if t.runtime != nil {
		definition.Version = "5"
		definition.Description += " python、python3、node 裸命令固定使用随应用提供的解释器，子进程 PATH 优先使用同套运行时。解释器和标准库目录在受限执行中只读；Python 字节码缓存和用户 site-packages 禁用。"
	}
	return definition
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
	if t.runtime != nil {
		if err := t.runtime.Check(ctx); err != nil {
			return nil, errors.Join(tool.ErrUnavailable, err)
		}
		if executable, ok := t.runtime.Resolve(a.Command[0]); ok {
			a.Command[0] = executable
		}
	}
	return prepare(ctx, scope, a, slices.Clone(t.environment), permissions, t.protection)
}
