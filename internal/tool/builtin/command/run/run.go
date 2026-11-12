// Package run 提供非交互命令执行，macOS 和 Windows 可采用服务端固定的系统沙箱边界。
package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		filtered = append(filtered, bundle.GitEnvironment()...)
		if !slices.Contains(protection.ReadOnlyDirectories, bundle.Directory()) {
			protection.ReadOnlyDirectories = append(protection.ReadOnlyDirectories, bundle.Directory())
		}
	}
	return &commandTool{environment: filtered, protection: protection, runtime: bundle}
}

func (t *commandTool) Definition() tool.Definition {
	definition := tool.Definition{Name: "run_command", Description: "执行非交互命令并返回退出码及有界 stdout/stderr。用 exit_code 结合子命令语义判断结果。command 为可执行文件和参数组成的数组，不自动套 Shell。cwd 的相对路径或省略值以工作区为基准；未绑定工作区时必须提供绝对 cwd。macOS 命令默认在只读沙箱执行，禁止网络，仅私有临时目录可写。需要在项目内运行写入命令时，用 sandbox_permissions=workspace_write 和 justification 请求本次沙箱内项目写入，须等待审批。确需沙箱外权限时使用 require_escalated 并说明理由。沙箱拒绝后不自动重试或提权。不支持交互输入、PTY 或持久后台会话。", Parameters: json.RawMessage(`{"type":"object","properties":{"command":{"type":"array","items":{"type":"string"},"minItems":1,"maxItems":256},"cwd":{"type":"string"},"sandbox_permissions":{"type":"string","enum":["use_default","workspace_write","require_escalated"]},"justification":{"type":"string"}},"required":["command"],"additionalProperties":false}`), Source: "builtin", Version: "3", Effects: []tool.Effect{tool.EffectUnknown}}
	if runtime.GOOS == "windows" {
		definition.Version = "4"
		definition.Description = "执行 Windows 非交互命令，command 为 exe/com 及参数数组，不自动套 Shell。PowerShell 或批处理脚本需显式指定 powershell.exe 或 cmd.exe；Python、JavaScript 脚本直接指定对应解释器。用 exit_code 判断成功。cwd 相对路径或省略值以工作区为基准，未绑定时必须提供绝对 cwd。默认使用 AppContainer 原生只读沙箱，网络关闭，本次私有临时目录和隔离配置存储可写。读取范围为项目、cwd、程序目录及 Windows 已授予访问的系统资源。读取其他位置可使用文件工具。需要命令修改项目时提交 sandbox_permissions=workspace_write 和非空 justification，等待本次审批。需要沙箱外权限时使用 require_escalated 并说明理由。沙箱建立或权限复核失败明确失败，拒绝后不自动重试或提权。不支持交互输入、PTY 或持久后台会话。"
	}
	if t.runtime != nil {
		definition.Version = "7"
		python, _ := t.runtime.Resolve("python3")
		node, _ := t.runtime.Resolve("node")
		git, _ := t.runtime.Resolve("git")
		// string 的 JSON 编码不会失败；显式编码可正确表示 Windows 分隔符及路径中的引号。
		pythonPath, _ := json.Marshal(python)
		nodePath, _ := json.Marshal(node)
		gitPath, _ := json.Marshal(git)
		definition.Description += fmt.Sprintf(`
内置 Python/Node 已就绪。运行 Python 或 JavaScript 时，默认直接以 command[0] 调用内置 python3 或 node，无需用户提醒，也无需先查找、探测或安装系统解释器。
直接调用示例：command=["python3","-c","print(1)"]、["python3","script.py"]、["node","-e","console.log(1)"]、["node","script.js"]；参数按数组逐项传入，不写成 Shell 命令字符串。
不要仅为运行 Python/Node 代码或脚本再套 bash、sh、zsh、powershell.exe 或 cmd.exe，尤其不要使用 bash -lc、sh -lc 或 zsh -lc；登录 Shell 可能重设 PATH，导致选到系统解释器。
确需管道、重定向等 Shell 语法时，才显式使用非登录 Shell，并按该 Shell 语法正确引用以下内置绝对入口，不依赖 Shell 再次查找解释器。入口以 JSON 字符串表示：python3=%s，node=%s。
用户明确要求项目虚拟环境或其他解释器时，使用指定解释器的显式路径，仍遵守相同的参数校验、审批和沙箱。python 兼容别名也固定到内置 Python，子进程 PATH 优先使用同套运行时。解释器和标准库目录在受限执行中只读；Python 字节码缓存和用户 site-packages 禁用。
Git 默认直接调用 command=["git","status","--short"] 等参数数组，固定使用内置 Git，无需探测或安装系统 Git。内置绝对入口（JSON 字符串）：git=%s。Git 辅助程序、模板与发行包配置随附，子进程禁用可选索引刷新、终端及 GCM 交互；凭据不足时明确失败。Git 的非零退出码按具体子命令语义判断（例如 diff --exit-code 的 1 表示存在差异）。读取仍遵守沙箱，写入 .git 的 init/add/commit 等操作需按现有规则请求沙箱外权限，clone/fetch/push 等网络访问也需审批。`, pythonPath, nodePath, gitPath)
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
