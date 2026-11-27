```go
package 运行

import (
	上下文包 "context"
	SHA256包 "crypto/sha256"
	JSON包 "encoding/json"
	错误判断包 "errors"
	输入输出包 "io"
	操作系统包 "os"
	文件路径包 "path/filepath"
	运行时包 "runtime"
	字符串包 "strings"
	测试包 "testing"
	时间包 "time"

	运行时资源包 "github.com/xiaoyu98628/callum/internal/runtimeassets"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

func 随附测试夹具(测试器 *测试包.测试器, 父级, 辅助 string) *运行时资源包.资源包 {
	测试器.标记测试辅助函数()
	目录 := 文件路径包.合并(父级, "resources with spaces")
	数据, 错误 := 操作系统包.读取文件(辅助)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	Python, 节点 := "python/bin/python3", "node/bin/node"
	if 运行时包.目标操作系统 == "windows" {
		Python, 节点 = "python/bin/python.exe", "node/bin/node.exe"
	}
	for _, 名称 := range []string{Python, 节点, "git/bin/git", "git/etc/gitconfig", "git/libexec/git-core/helper", "git/share/templates/template"} {
		完整 := 文件路径包.合并(目录, 文件路径包.来源斜杠(名称))
		if 错误 := 操作系统包.创建目录全部(文件路径包.目录(完整), 0755); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		if 错误 := 操作系统包.写入文件(完整, 数据, 0755); 错误 != nil {
			测试器.报告致命错误(错误)
		}
	}
	文件集, 错误 := 运行时资源包.资源库存(测试器.上下文(), 目录)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	清单 := 运行时资源包.资源清单{格式版本: 运行时资源包.格式版本, 平台: 运行时包.目标操作系统, 架构: 运行时包.目标架构, Python: 运行时资源包.可执行文件{版本: "controlled", 路径: Python}, 节点: 运行时资源包.可执行文件{版本: "controlled", 路径: 节点}, Git: 运行时资源包.Git运行时{可执行文件: 运行时资源包.可执行文件{版本: "controlled", 路径: "git/bin/git"}, 进程执行路径: "git/libexec/git-core", 模板路径: "git/share/templates", 配置路径: "git/etc/gitconfig"}, 文件集: 文件集}
	if 错误 := 运行时资源包.写入清单(目录, 清单); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	资源包, 错误 := 运行时资源包.加载(测试器.上下文(), 目录)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	return 资源包
}

func Test随附命令集使用固定可执行文件集并且保留取消(测试器 *测试包.测试器) {
	资源包 := 随附测试夹具(测试器, 测试器.创建临时目录(), 构建辅助(测试器))
	实例 := 新建带有运行时([]string{"PATH=" + 文件路径包.合并(测试器.创建临时目录(), "missing"), "OPENAI_API_KEY=must-not-leak"}, 工具包.受保护数据{}, 资源包).(*命令工具)
	for _, 名称 := range []string{"python", "python3", "node", "git"} {
		原始, 错误 := JSON包.序列化(参数{命令: []string{名称, "inspect", "a b", "$HOME"}, 工作目录: 测试器.创建临时目录()})
		if 错误 != nil {
			测试器.报告致命错误(错误)
		}
		调用, 错误 := 实例.准备带有权限(测试器.上下文(), 工具包.范围{}, 原始, 工具包.执行权限{沙箱模式: 工具包.沙箱完整访问})
		if 错误 != nil {
			测试器.报告致命错误(错误)
		}
		固定 := 调用.(*工具调用).固定
		if !字符串包.存在前缀(固定.可执行文件.实际, 资源包.获取目录()+string(文件路径包.分隔符)) || 环境值(固定.环境, "PYTHONDONTWRITEBYTECODE") != "1" {
			测试器.报告致命错误("未固定内置解释器或未隔离缓存")
		}
		结果, 执行错误 := 调用.执行(测试器.上下文())
		if 错误 := 错误判断包.合并(执行错误, 调用.关闭()); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		报告 := 解码报告(测试器, 结果)
		if 报告.退出码 == nil || *报告.退出码 != 0 || !报告.清理完成 || 字符串包.包含(string(结果.输出), "must-not-leak") || !字符串包.包含(报告.标准输出.数据, "$HOME") {
			测试器.格式化报告致命错误("命令映射改变了参数、环境或生命周期: %s", 结果.输出)
		}
	}
	原始, _ := JSON包.序列化(参数{命令: []string{"python", "sleep"}, 工作目录: 测试器.创建临时目录()})
	调用, 错误 := 实例.准备带有权限(测试器.上下文(), 工具包.范围{}, 原始, 工具包.执行权限{沙箱模式: 工具包.沙箱完整访问})
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	执行上下文, 取消函数 := 上下文包.创建超时上下文(测试器.上下文(), 100*时间包.毫秒)
	defer 取消函数()
	结果, 错误 := 调用.执行(执行上下文)
	if !错误判断包.是否属于(错误, 上下文包.超出截止时间错误) || !解码报告(测试器, 结果).清理完成 {
		测试器.格式化报告致命错误("内置命令没有按超时回收: %s %v", 结果.输出, 错误)
	}
	if 错误 := 调用.关闭(); 错误 != nil {
		测试器.报告致命错误(错误)
	}
}

// 显式传入本次准备的工作区资源，普通单元测试和 CI 不依赖用户安装的解释器。
func Test随附解释器集(测试器 *测试包.测试器) {
	目录 := 操作系统包.获取环境变量("CALLUM_TEST_RUNTIME_DIR")
	if 目录 == "" {
		测试器.跳过("需要本次准备的独立运行时资源")
	}
	清单文件, 错误 := 操作系统包.打开(文件路径包.合并(目录, "manifest.json"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	// 版本断言与命令实际使用的运行时绑定到同一份清单。
	摘要 := SHA256包.新建()
	清单, 解码错误 := 运行时资源包.解码(输入输出包.分流读取器(清单文件, 摘要))
	if 错误 := 错误判断包.合并(解码错误, 清单文件.关闭()); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	资源包, 错误 := 运行时资源包.加载带有清单SHA256(测试器.上下文(), 目录, [SHA256包.大小]byte(摘要.校验和(nil)))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	实例 := 新建带有运行时([]string{"PATH=" + 测试器.创建临时目录(), "OPENAI_API_KEY=must-not-leak"}, 工具包.受保护数据{目录: 测试器.创建临时目录()}, 资源包).(*命令工具)
	Python := `
import json, os, sqlite3, ssl, subprocess, sys

actual_version = ".".join(map(str, sys.version_info[:3]))
assert actual_version == sys.argv[1], f"Python version: got {actual_version}, want {sys.argv[1]}"
assert os.getenv("OPENAI_API_KEY") is None
assert sqlite3.connect(":memory:").execute("select 1").fetchone() == (1,)
ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
assert subprocess.check_output(["python3", "-c", "print('child-ok')"], text=True).strip() == "child-ok"
print("bundled-python-ok")
`
	节点 := `const assert=require('node:assert/strict'); require('node:fs'); require('node:crypto').randomBytes(4); assert.equal(process.version,'v26.13.0'); assert.equal(process.env.OPENAI_API_KEY,undefined); assert.equal(require('node:child_process').execFileSync('node',['-p',"'child-ok'"],{encoding:'utf8'}).trim(),'child-ok'); console.log('bundled-node-ok')`
	命令集 := [][]string{{"python3", "-c", Python, 清单.Python.版本}, {"node", "-e", 节点}, {"python3", "-c", "print('x'*100000)"}}
	if 运行时包.目标操作系统 != "windows" {
		Python路径, _ := 资源包.解析("python3")
		节点路径, _ := 资源包.解析("node")
		Git路径, _ := 资源包.解析("git")
		// 路径 只有临时空目录；必要的管道和重定向仍可通过显式绝对入口执行。
		脚本 := `"$1" -c 'print("pipe-ok")' | "$2" -e 'let s=""; process.stdin.on("data",c=>s+=c); process.stdin.on("end",()=>{require("node:assert/strict").equal(s.trim(),"pipe-ok");console.log("bundled-pipe-ok")})' && "$3" --version > "$TMPDIR/git-version" && "$1" -c 'import os; from pathlib import Path; assert Path(os.environ["TMPDIR"],"git-version").read_text().startswith("git version 2.56.1"); print("bundled-shell-ok")'`
		命令集 = append(命令集, []string{"/bin/sh", "-c", 脚本, "callum-shell", Python路径, 节点路径, Git路径})
	}
	for _, 命令 := range 命令集 {
		原始, _ := JSON包.序列化(参数{命令: 命令, 工作目录: 测试器.创建临时目录()})
		调用, 错误 := 实例.准备带有权限(测试器.上下文(), 工具包.范围{}, 原始, 工具包.执行权限{沙箱模式: 工具包.沙箱只读})
		if 错误 != nil {
			测试器.报告致命错误(错误)
		}
		结果, 执行错误 := 调用.执行(测试器.上下文())
		if 错误 := 错误判断包.合并(执行错误, 调用.关闭()); 错误 != nil {
			测试器.格式化报告致命错误("真实解释器执行失败: %s %v", 结果.输出, 错误)
		}
		报告 := 解码报告(测试器, 结果)
		if 报告.退出码 == nil || *报告.退出码 != 0 || !报告.清理完成 {
			测试器.格式化报告致命错误("真实解释器未正常完成: %s", 结果.输出)
		}
		if 字符串包.包含(命令[2], "100000") && !报告.标准输出.已截取 {
			测试器.报告致命错误("解释器的大量输出未保持有界")
		}
		if 命令[0] == "/bin/sh" && (!字符串包.包含(报告.标准输出.数据, "bundled-pipe-ok") || !字符串包.包含(报告.标准输出.数据, "bundled-shell-ok")) {
			测试器.格式化报告致命错误("必要 Shell 没有使用内置程序完成管道和重定向: %s", 结果.输出)
		}
	}
}

// 使用本次准备的 Git 验证真实仓库操作，所有写入限定在测试临时目录。
func Test随附Git(测试器 *测试包.测试器) {
	目录 := 操作系统包.获取环境变量("CALLUM_TEST_RUNTIME_DIR")
	if 目录 == "" {
		测试器.跳过("需要本次准备的独立运行时资源")
	}
	资源包, 错误 := 运行时资源包.加载(测试器.上下文(), 目录)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	家目录 := 测试器.创建临时目录()
	实例 := 新建带有运行时([]string{"PATH=" + 测试器.创建临时目录(), "HOME=" + 家目录, "USERPROFILE=" + 家目录}, 工具包.受保护数据{目录: 测试器.创建临时目录()}, 资源包).(*命令工具)
	仓储 := 测试器.创建临时目录()
	执行Git := func(模式 工具包.沙箱模式, 参数 ...string) 进程报告 {
		测试器.标记测试辅助函数()
		原始, _ := JSON包.序列化(参数{命令: append([]string{"git"}, 参数...), 工作目录: 仓储})
		调用, 错误 := 实例.准备带有权限(测试器.上下文(), 工具包.范围{}, 原始, 工具包.执行权限{沙箱模式: 模式})
		if 错误 != nil {
			测试器.报告致命错误(错误)
		}
		结果, 执行错误 := 调用.执行(测试器.上下文())
		if 错误 := 错误判断包.合并(执行错误, 调用.关闭()); 错误 != nil {
			测试器.格式化报告致命错误("Git 执行失败: %s %v", 结果.输出, 错误)
		}
		报告 := 解码报告(测试器, 结果)
		if !报告.清理完成 || 报告.退出码 == nil {
			测试器.格式化报告致命错误("Git 未正常回收: %s", 结果.输出)
		}
		return 报告
	}
	成功 := func(模式 工具包.沙箱模式, 参数 ...string) string {
		测试器.标记测试辅助函数()
		报告 := 执行Git(模式, 参数...)
		if *报告.退出码 != 0 {
			测试器.格式化报告致命错误("git %v: exit=%d stdout=%s stderr=%s", 参数, *报告.退出码, 报告.标准输出.数据, 报告.标准错误输出.数据)
		}
		return 报告.标准输出.数据
	}
	成功(工具包.沙箱只读, "--version")
	// HTTPS 辅助程序在空 标准输入 下以 1 结束，不连接端点；查找或依赖加载失败会有诊断。
	远程 := 执行Git(工具包.沙箱只读, "remote-https", "origin", "https://example.invalid/not-accessed")
	if *远程.退出码 != 1 || 远程.标准输出.数据 != "" || 远程.标准错误输出.数据 != "" {
		测试器.格式化报告致命错误("HTTPS 辅助程序未正常处理 EOF: %+v", 远程)
	}
	成功(工具包.沙箱只读, "lfs", "version")
	if 实际值 := 字符串包.裁剪空间(成功(工具包.沙箱只读, "--exec-path")); !字符串包.存在前缀(实际值, 资源包.获取目录()+string(文件路径包.分隔符)) {
		测试器.格式化报告致命错误("辅助程序没有固定: %s", 实际值)
	}
	成功(工具包.沙箱完整访问, "init", "--initial-branch=main")
	if _, 错误 := 操作系统包.状态信息(文件路径包.合并(仓储, ".git", "hooks", "pre-commit.sample")); 错误 != nil {
		测试器.报告致命错误("没有使用随附 Git 模板:", 错误)
	}
	成功(工具包.沙箱完整访问, "config", "user.name", "Callum Test")
	成功(工具包.沙箱完整访问, "config", "user.email", "callum-test@example.invalid")
	成功(工具包.沙箱完整访问, "config", "commit.gpgsign", "false")
	文件 := 文件路径包.合并(仓储, "tracked.txt")
	if 错误 := 操作系统包.写入文件(文件, []byte("before\n"), 0600); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	成功(工具包.沙箱完整访问, "add", "tracked.txt")
	成功(工具包.沙箱完整访问, "commit", "-m", "controlled commit")
	if !字符串包.包含(成功(工具包.沙箱只读, "log", "-1", "--format=%s"), "controlled commit") {
		测试器.报告致命错误("无法读取提交")
	}
	if 错误 := 操作系统包.写入文件(文件, []byte("after\n"), 0600); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	索引 := 文件路径包.合并(仓储, ".git", "index")
	之前, 错误 := 操作系统包.读取文件(索引)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	信息, 错误 := 操作系统包.状态信息(索引)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if !字符串包.包含(成功(工具包.沙箱只读, "status", "--short"), "tracked.txt") {
		测试器.报告致命错误("状态没有报告修改")
	}
	之后, 错误 := 操作系统包.读取文件(索引)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	之后信息, 错误 := 操作系统包.状态信息(索引)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if string(之前) != string(之后) || !信息.模块时间().相等(之后信息.模块时间()) {
		测试器.报告致命错误("只读 status 修改了索引")
	}
	if !字符串包.包含(成功(工具包.沙箱只读, "diff", "--no-ext-diff"), "+after") {
		测试器.报告致命错误("无法读取差异")
	}
	if 代码 := *执行Git(工具包.沙箱只读, "diff", "--exit-code", "--no-ext-diff").退出码; 代码 != 1 {
		测试器.格式化报告致命错误("差异退出码丢失: %d", 代码)
	}
	if 运行时包.目标操作系统 == "darwin" || 运行时包.目标操作系统 == "windows" {
		if *执行Git(工具包.沙箱只读, "add", "tracked.txt").退出码 == 0 {
			测试器.报告致命错误("只读沙箱允许修改 .git")
		}
	}
	复制 := 文件路径包.合并(测试器.创建临时目录(), "local clone")
	成功(工具包.沙箱完整访问, "clone", "--no-local", "--no-hardlinks", 仓储, 复制)
	if 数据, 错误 := 操作系统包.读取文件(文件路径包.合并(复制, "tracked.txt")); 错误 != nil || string(数据) != "before\n" {
		测试器.格式化报告致命错误("本地克隆内容错误: %q %v", 数据, 错误)
	}
	// 路径 内没有系统程序，验证解释器子进程同样找到内置 Git。
	Python := `import subprocess; print(subprocess.check_output(["git","--version"],text=True))`
	原始, _ := JSON包.序列化(参数{命令: []string{"python3", "-c", Python}, 工作目录: 仓储})
	调用, 错误 := 实例.准备带有权限(测试器.上下文(), 工具包.范围{}, 原始, 工具包.执行权限{沙箱模式: 工具包.沙箱只读})
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	结果, 执行错误 := 调用.执行(测试器.上下文())
	if 错误 := 错误判断包.合并(执行错误, 调用.关闭()); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 报告 := 解码报告(测试器, 结果); 报告.退出码 == nil || *报告.退出码 != 0 || !字符串包.包含(报告.标准输出.数据, "git version 2.56.1") {
		测试器.格式化报告致命错误("子进程没有使用内置 Git: %s", 结果.输出)
	}
}
```
