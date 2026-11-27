```go
//go:build darwin

package 运行

import (
	上下文包 "context"
	随机包 "crypto/rand"
	错误判断包 "errors"
	格式化包 "fmt"
	操作系统包 "os"
	文件路径包 "path/filepath"
	切片包 "slices"
	字符串包 "strings"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

// 配置沙箱 只固定边界、路径及系统启动器，不创建文件或运行探测命令。
func (当前固定命令 *固定命令) 配置沙箱(当前参数 参数, 权限 工具包.执行权限, 保护 工具包.受保护数据) error {
	项目写入, 已处理, 错误 := 当前固定命令.沙箱请求(当前参数, 权限)
	if 错误 != nil || 已处理 {
		return 错误
	}
	当前固定命令.沙箱可执行文件, 错误 = 固定目标路径("/usr/bin/sandbox-exec", false)
	if 错误 != nil {
		return 错误判断包.合并(工具包.不可用错误, 错误)
	}
	当前固定命令.临时父级, 错误 = 固定目标路径(操作系统包.创建临时目录(), true)
	if 错误 != nil {
		return 错误
	}
	if 保护.目录 != "" {
		当前固定命令.受保护目录, 错误 = 固定目标路径(保护.目录, true)
		if 错误 != nil {
			return 错误
		}
		if 保护.包含(当前固定命令.临时父级.实际) || (项目写入 && 保护.包含(当前固定命令.工作区.实际)) {
			return 工具包.已拒绝错误
		}
	}
	当前固定命令.临时路径 = 文件路径包.合并(当前固定命令.临时父级.实际, "callum-command-"+随机包.文本())
	当前固定命令.边界.沙箱内, 当前固定命令.边界.模式 = true, 工具包.沙箱只读
	if 项目写入 {
		当前固定命令.边界.模式 = 工具包.沙箱工作区写入
		当前固定命令.边界.创建工作区目录 = 当前固定命令.创建工作区目录()
	}
	当前固定命令.配置档 = Seatbelt配置档(项目写入, 当前固定命令.受保护目录 != nil)
	if 当前固定命令.受保护目录 != nil {
		// 拒绝 优先于 允许，必须从原有数据读取拒绝规则中排除固定的只读技能子树。
		var 排除项集 字符串包.构建器
		for 索引, 根 := range 当前固定命令.读取仅目录集 {
			if (工具包.受保护数据{目录: 当前固定命令.受保护目录.实际}).包含(根.实际) {
				排除项集.写入字符串(格式化包.格式化字符串(" (require-not (subpath (param \"RUNTIME_ROOT_%d\")))", 索引))
			}
		}
		if 排除项集.长度() > 0 {
			// 原生程序解析资源绝对路径需要查询父目录元数据；根目录枚举仍单独拒绝。
			当前固定命令.配置档 = 字符串包.替换(当前固定命令.配置档, `(deny file-read* (subpath (param "APP_DATA")))`, `(deny file-read* (require-all (subpath (param "APP_DATA")) (require-not (literal (param "APP_DATA")))`+排除项集.字符串()+`))`+"\n"+`(deny file-read-data (literal (param "APP_DATA")))`, 1)
		}
	}
	祖先参数 := 当前固定命令.保护数据祖先集(项目写入)
	当前固定命令.启动参数 = []string{当前固定命令.沙箱可执行文件.实际, "-p", 当前固定命令.配置档, "-D", "COMMAND_TEMP=" + 当前固定命令.临时路径}
	当前固定命令.启动参数 = append(当前固定命令.启动参数, 祖先参数...)
	if 当前固定命令.工作目录 != nil {
		当前固定命令.配置档 += `(allow file-write* (require-all (subpath (param "RUN_WORK")) (require-not (regex #"(^|/)[.](git|codex|agents|aws)(/|$)"))))` + "\n"
		当前固定命令.启动参数 = append(当前固定命令.启动参数, "-D", "RUN_WORK="+当前固定命令.工作目录.实际)
	}
	for 索引, 根 := range 当前固定命令.读取仅目录集 {
		键 := 格式化包.格式化字符串("RUNTIME_ROOT_%d", 索引)
		当前固定命令.配置档 += 格式化包.格式化字符串("(deny file-write* (subpath (param \"%s\")))\n", 键)
		当前固定命令.启动参数 = append(当前固定命令.启动参数, "-D", 键+"="+根.实际)
	}
	// 策略在所有只读资源规则拼接完成后传给启动器。
	当前固定命令.启动参数[2] = 当前固定命令.配置档
	if 当前固定命令.受保护目录 != nil {
		当前固定命令.启动参数 = append(当前固定命令.启动参数, "-D", "APP_DATA="+当前固定命令.受保护目录.实际)
	}
	if 项目写入 {
		当前固定命令.启动参数 = append(当前固定命令.启动参数, "-D", "WORKSPACE="+当前固定命令.工作区.实际)
	}
	当前固定命令.启动参数 = append(当前固定命令.启动参数, "--")
	当前固定命令.启动参数 = append(当前固定命令.启动参数, 当前固定命令.命令...)
	// 不把真实用户缓存目录自动加入写入范围。常用工具缓存转入此次受控临时目录。
	当前固定命令.环境 = 沙箱环境(当前固定命令.环境, 当前固定命令.临时路径)
	return nil
}

// 仅识别系统 创建目录 在项目内创建单层目录的操作，现有目标不会被覆盖。
func (当前固定命令 *固定命令) 创建工作区目录() bool {
	if len(当前固定命令.命令) != 2 || 当前固定命令.命令[0] != "/bin/mkdir" {
		return false
	}
	名称 := 当前固定命令.命令[1]
	if !文件路径包.是否本地路径(名称) || 文件路径包.基准(名称) != 名称 || 名称 == "." || 字符串包.存在前缀(名称, "-") {
		return false
	}
	相对, 错误 := 文件路径包.相对路径(当前固定命令.工作区.实际, 当前固定命令.目录.实际)
	return 错误 == nil && 文件路径包.是否本地路径(相对)
}

// 保护项目内通往应用数据的祖先路径，避免获批命令通过重命名祖先绕过路径限制。
func (当前固定命令 *固定命令) 保护数据祖先集(项目写入 bool) []string {
	if !项目写入 {
		return nil
	}
	var 参数 []string
	根目录集 := 切片包.复制(当前固定命令.读取仅目录集)
	if 当前固定命令.受保护目录 != nil {
		根目录集 = append(根目录集, 当前固定命令.受保护目录)
	}
	for _, 根 := range 根目录集 {
		for 父级 := 文件路径包.目录(根.实际); ; 父级 = 文件路径包.目录(父级) {
			相对, 错误 := 文件路径包.相对路径(当前固定命令.工作区.实际, 父级)
			if 错误 != nil || 相对 == "." || !文件路径包.是否本地路径(相对) {
				break
			}
			键 := 格式化包.格式化字符串("APP_DATA_PARENT_%d", len(参数)/2)
			当前固定命令.配置档 += 格式化包.格式化字符串("(deny file-write-unlink (literal (param \"%s\")))\n", 键)
			参数 = append(参数, "-D", 键+"="+父级)
		}
	}
	return 参数
}

// 初始化执行 只在审批完成、路径复核后创建私有临时目录；调用者在进程树收尾后清理。
func (当前固定命令 *固定命令) 初始化执行(上下文包.上下文) (func() error, error) {
	if !当前固定命令.边界.沙箱内 {
		return func() error { return nil }, nil
	}
	if 错误 := 操作系统包.创建目录(当前固定命令.临时路径, 0700); 错误 != nil {
		return nil, 错误判断包.合并(工具包.不可用错误, 错误)
	}
	return func() error { return 操作系统包.移除全部(当前固定命令.临时路径) }, nil
}

// 路径使用 沙箱-进程执行 的独立参数传递，不能将带引号的目录拼进策略代码。
// 默认不开放任何网络或其他应用的 IPC；子进程继承同一策略。
func Seatbelt配置档(工作区写入, 保护数据 bool) string {
	配置档 := `(version 1)
(deny default)
(allow process-exec)
(allow process-fork)
(allow signal (target same-sandbox))
(allow process-info* (target same-sandbox))
(allow file-read*)
(allow sysctl-read)
(allow mach-lookup (global-name "com.apple.system.opendirectoryd.libinfo"))
(allow file-write-data (require-all (literal "/dev/null") (vnode-type CHARACTER-DEVICE)))
(allow file-write* (require-all
  (subpath (param "COMMAND_TEMP"))
  (require-not (regex #"(^|/)[.](git|codex|agents|aws)(/|$)"))))
`
	if 工作区写入 {
		配置档 += `(allow file-write* (require-all
  (subpath (param "WORKSPACE"))
  (require-not (regex #"(^|/)[.](git|codex|agents|aws)(/|$)"))))
(deny file-write-unlink (literal (param "WORKSPACE")))
`
	}
	配置档 += `(deny file-write-unlink (literal (param "COMMAND_TEMP")))
(deny file-write* (regex #"(^|/)[.](git|codex|agents|aws)(/|$)"))
`
	if 保护数据 {
		配置档 += `(deny file-read* (subpath (param "APP_DATA")))
(deny file-write* (subpath (param "APP_DATA")))
`
	}
	return 配置档
}

func (*固定命令) 沙箱后端() string { return "macos-seatbelt" }
```
