```go
// 包 运行 提供非交互命令执行，macOS 和 Windows 可采用服务端固定的系统沙箱边界。
package 运行

import (
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"
	格式化包 "fmt"
	文件路径包 "path/filepath"
	运行时包 "runtime"
	切片包 "slices"
	字符串包 "strings"

	运行时资源包 "github.com/xiaoyu98628/callum/internal/runtimeassets"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

type 命令工具 struct {
	环境  []string
	保护  工具包.受保护数据
	运行时 *运行时资源包.资源包
}

// 新建 从装配层传入的环境中固定非敏感基础变量；不读取或修改全局环境。
func 新建(环境 []string, 保护 工具包.受保护数据) 工具包.工具 {
	return 新建带有运行时(环境, 保护, nil)
}

// 新建带有运行时 借用已校验资源，只设置命令子进程环境，不改变服务进程环境。
func 新建带有运行时(环境 []string, 保护 工具包.受保护数据, 资源包 *运行时资源包.资源包) 工具包.工具 {
	已筛选 := 基准环境(环境)
	保护.只读目录集 = 切片包.复制(保护.只读目录集)
	if 资源包 != nil {
		路径集 := append(资源包.二进制目录集(), 文件路径包.拆分列表(环境值(已筛选, "PATH"))...)
		已筛选 = 切片包.删除函数(已筛选, func(条目 string) bool { return 字符串包.存在前缀(条目, "PATH=") })
		已筛选 = append(已筛选, "PATH="+字符串包.合并(路径集, string(文件路径包.列表分隔符)), "PYTHONDONTWRITEBYTECODE=1", "PYTHONNOUSERSITE=1", "PYTHONUTF8=1")
		已筛选 = append(已筛选, 资源包.Git环境()...)
		已筛选 = append(已筛选, 资源包.工作区环境()...)
		if !切片包.包含(保护.只读目录集, 资源包.获取目录()) {
			保护.只读目录集 = append(保护.只读目录集, 资源包.获取目录())
		}
	}
	return &命令工具{环境: 已筛选, 保护: 保护, 运行时包: 资源包}
}

func (当前命令工具 *命令工具) 获取定义() 工具包.定义 {
	定义 := 工具包.定义{名称: "run_command", 说明: "执行非交互命令并返回退出码及有界 stdout/stderr。用 exit_code 结合子命令语义判断结果。command 为可执行文件和参数组成的数组，不自动套 Shell。cwd 的相对路径或省略值以工作区为基准；未绑定工作区时必须提供绝对 cwd。不支持交互输入、PTY 或持久后台会话。", 参数: JSON包.原始JSON消息(`{"type":"object","properties":{"command":{"type":"array","items":{"type":"string"},"minItems":1,"maxItems":256},"cwd":{"type":"string"},"sandbox_permissions":{"type":"string","enum":["use_default","workspace_write","require_escalated"]},"justification":{"type":"string"}},"required":["command"],"additionalProperties":false}`), 来源: "builtin", 版本: "9", 影响集: []工具包.影响{工具包.影响未知}}
	switch 运行时包.目标操作系统 {
	case "darwin":
		定义.说明 += "本次设置的 sandbox_mode 为 read-only 或 workspace-write 时，use_default 在 macOS 只读系统沙箱中执行，禁止网络，仅本轮私有临时目录和工作目录可写。仅当设置为 workspace-write 且已绑定工作区时，可以用 sandbox_permissions=workspace_write 和 justification 申请本次沙箱内项目写入，执行前仍需审批。"
	case "windows":
		定义.说明 += "Windows 的 command 应为 exe/com 及参数数组；PowerShell 或批处理脚本需显式指定 powershell.exe 或 cmd.exe，Python、JavaScript 脚本直接指定解释器。本次设置的 sandbox_mode 为 read-only 或 workspace-write 时，use_default 使用 AppContainer 只读沙箱，网络关闭，本轮私有目录和隔离配置存储可写。读取范围为项目、cwd、程序目录及系统已授予访问的资源，其他位置可使用文件工具。仅当设置为 workspace-write 且已绑定工作区时，可以用 sandbox_permissions=workspace_write 和 justification 申请本次沙箱内项目写入，执行前仍需审批。沙箱建立或权限复核失败明确失败。"
	default:
		定义.说明 += "当前平台尚未接入命令系统沙箱，不得声称命令已受文件或网络沙箱隔离。受限模式下命令由服务端按实际边界逐次判定审批；workspace_write 命令沙箱不可用。"
	}
	定义.说明 += `
实际执行模式以服务端提供的本次设置快照为准。danger-full-access 使用当前账户权限，不建立系统沙箱；历史快照未记录 sandbox_mode 时不推断默认沙箱，仍由服务端校验和审批。受限模式下确需沙箱外权限时使用 require_escalated 并说明理由。参数不能修改服务端设置、扩大已有授权或绕过审批；拒绝后不自动重试或提权。
执行前会拒绝仅包装普通程序或用分号串联独立命令的 sh/bash -c，返回的参数错误需按说明纠正，不要改换 Shell 绕过。command 必须是真正的字符串数组，不能是包含数组文本的字符串。
命令选择规则：单个命令直接使用可执行程序和参数数组；多个独立操作分别发起工具调用。不要仅为用 echo/printf 输出分隔标题、串联独立命令或查看多个目录而构造 sh -c 等 Shell 脚本。查看目录优先使用 list_directory，通过 path 指定目录、limit 限制条目。例如查看 Git 状态并列出目录时，分别调用 command=["git","status","--short"] 和 list_directory。
确需管道、重定向、条件控制等 Shell 语法时，才显式使用当前平台的非登录 Shell；Windows 使用 PowerShell -NoProfile -NonInteractive 或 cmd /d。Shell 脚本仍需正确引用程序路径和参数，并遵守同样的审批与沙箱边界。`
	if 当前命令工具.运行时 != nil {
		Python, _ := 当前命令工具.运行时.解析("python3")
		节点, _ := 当前命令工具.运行时.解析("node")
		Git, _ := 当前命令工具.运行时.解析("git")
		// string 的 JSON 编码不会失败；显式编码可正确表示 Windows 分隔符及路径中的引号。
		Python路径, _ := JSON包.序列化(Python)
		节点路径, _ := JSON包.序列化(节点)
		Git路径, _ := JSON包.序列化(Git)
		定义.说明 += 格式化包.格式化字符串(`
内置 Python/Node/Git 已就绪。默认直接以 command[0] 调用内置 python3、node 或 git，无需用户提醒，也无需先查找、探测或安装系统版本。
直接调用示例：command=["python3","-c","print(1)"]、["python3","script.py"]、["node","-e","console.log(1)"]、["node","script.js"]、["git","status","--short"]；参数按数组逐项传入，不写成 Shell 命令字符串。
不要仅为运行单个 Python/Node/Git 命令或拼接独立操作再套 bash、sh、zsh、powershell.exe 或 cmd.exe，尤其不要使用 bash -lc、sh -lc 或 zsh -lc；登录 Shell 可能重设 PATH，导致选到系统版本。
确需 Shell 语法时，按相应 Shell 的引用规则使用以下内置绝对入口，不依赖 PATH 再次查找。Shell 字符串里的程序不会经过裸命令固定映射。入口以 JSON 字符串表示：python3=%s，node=%s，git=%s。
用户明确要求项目虚拟环境或其他解释器时，使用指定解释器的显式路径，仍遵守相同的参数校验、审批和沙箱。python 兼容别名也固定到内置 Python，子进程 PATH 优先使用同套运行时。解释器和标准库目录在受限执行中只读；Python 字节码缓存和用户 site-packages 禁用。
Git 辅助程序、模板与发行包配置随附，子进程禁用可选索引刷新、终端及 GCM 交互；凭据不足时明确失败。Git 的非零退出码按具体子命令语义判断（例如 diff --exit-code 的 1 表示存在差异）。受限模式下读取遵守沙箱，写入 .git 的 init/add/commit 等操作需按现有规则请求沙箱外权限，clone/fetch/push 等网络访问也需审批；danger-full-access 按服务端固定的完全访问设置执行，仍不构成对提交、推送等操作的用户意图授权。`, Python路径, 节点路径, Git路径)
	}
	return 定义
}

func (*命令工具) 校验(原始 JSON包.原始JSON消息) (JSON包.原始JSON消息, error) {
	甲, 错误 := 解码参数(原始)
	if 错误 != nil {
		return nil, 错误
	}
	return JSON包.序列化(甲)
}

func (当前命令工具 *命令工具) 准备(执行上下文 上下文包.上下文, 范围 工具包.范围, 原始 JSON包.原始JSON消息) (工具包.工具调用, error) {
	return 当前命令工具.准备带有权限(执行上下文, 范围, 原始, 工具包.执行权限{})
}

func (当前命令工具 *命令工具) 准备带有权限(执行上下文 上下文包.上下文, 范围 工具包.范围, 原始 JSON包.原始JSON消息, 权限 工具包.执行权限) (工具包.工具调用, error) {
	甲, 错误 := 解码参数(原始)
	if 错误 != nil {
		return nil, 错误
	}
	if 当前命令工具.运行时 != nil {
		if 错误 := 当前命令工具.运行时.检查(执行上下文); 错误 != nil {
			return nil, 错误判断包.合并(工具包.不可用错误, 错误)
		}
		if 可执行文件, 成功 := 当前命令工具.运行时.解析(甲.命令[0]); 成功 {
			甲.命令[0] = 可执行文件
		}
	}
	保护 := 当前命令工具.保护
	保护.只读目录集 = 切片包.复制(保护.只读目录集)
	if 权限.输入目录 != "" {
		保护.只读目录集 = append(保护.只读目录集, 权限.输入目录)
	}
	环境 := 切片包.复制(当前命令工具.环境)
	if 权限.工作目录 != "" {
		环境 = append(环境, "CALLUM_RUN_WORK_DIR="+权限.工作目录)
	}
	return 准备模块(执行上下文, 范围, 甲, 环境, 权限, 保护)
}
```
