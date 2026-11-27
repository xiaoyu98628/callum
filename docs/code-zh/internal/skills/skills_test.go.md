```go
package 技能

import (
	上下文包 "context"
	错误判断包 "errors"
	输入输出包 "io"
	结构化日志包 "log/slog"
	操作系统包 "os"
	文件路径包 "path/filepath"
	运行时包 "runtime"
	字符串包 "strings"
	测试包 "testing"
)

func 新建存储(测试器 *测试包.测试器) *存储 {
	测试器.标记测试辅助函数()
	值, 错误 := 打开(测试器.上下文(), 测试器.创建临时目录(), 结构化日志包.新建(结构化日志包.新建文本处理器(输入输出包.丢弃, nil)))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	return 值
}

func 写入技能(测试器 *测试包.测试器, 存储 *存储, 目录, 内容 string) string {
	测试器.标记测试辅助函数()
	路径 := 文件路径包.合并(存储.获取目录(), 目录, "SKILL.md")
	if 错误 := 操作系统包.创建目录全部(文件路径包.目录(路径), 0700); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 操作系统包.写入文件(路径, []byte(内容), 0600); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	return 路径
}

func Test发现刷新用户技能并且保留来源集(测试器 *测试包.测试器) {
	存储 := 新建存储(测试器)
	if 值集, 错误 := 存储.快照(测试器.上下文()); 错误 != nil || len(值集) != 0 {
		测试器.格式化报告致命错误("空目录: %v %v", 值集, 错误)
	}
	路径 := 写入技能(测试器, 存储, "custom", "---\nname: \"Custom Skill\"\ndescription: >-\n  Analyze logs\n  when requested.\nmetadata:\n  short-description: logs\n---\nprivate-body-marker")
	值集, 错误 := 存储.快照(测试器.上下文())
	if 错误 != nil || len(值集) != 1 || 值集[0].说明 != "Analyze logs when requested." || 值集[0].路径 != 路径 {
		测试器.格式化报告致命错误("未识别通用技能: %+v %v", 值集, 错误)
	}
	写入技能(测试器, 存储, "nested/another", "---\nname: Custom Skill\ndescription: another source\n---\n")
	if 值集, 错误 := 存储.快照(测试器.上下文()); 错误 != nil || len(值集) != 2 {
		测试器.格式化报告致命错误("同名技能被覆盖: %+v %v", 值集, 错误)
	}
	写入技能(测试器, 存储, "custom", "---\nname: Custom Skill\ndescription: updated description\n---\nprivate-body-marker")
	if 值集, 错误 := 存储.快照(测试器.上下文()); 错误 != nil || 值集[0].说明 != "updated description" {
		测试器.格式化报告致命错误("技能元数据修改未刷新: %+v %v", 值集, 错误)
	}
	if 错误 := 操作系统包.创建目录全部(文件路径包.合并(存储.获取目录(), "custom", "agents"), 0700); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 操作系统包.写入文件(文件路径包.合并(存储.获取目录(), "custom", "agents", "callum.yaml"), []byte("policy:\n  allow_implicit_invocation: false\n"), 0600); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	上下文文本, 错误 := 存储.上下文(测试器.上下文())
	if 错误 != nil || 字符串包.包含(上下文文本, "private-body-marker") || !字符串包.包含(上下文文本, `"explicit_only":true`) {
		测试器.格式化报告致命错误("没有按需加载或策略丢失: %s %v", 上下文文本, 错误)
	}
	引导内容, _, 已找到 := 字符串包.截断(上下文文本, "\n[")
	if !已找到 || !字符串包.包含(引导内容, "# Using skills") || !字符串包.包含(引导内容, "Open the skill's SKILL.md with read_file") || 字符串包.包含(字符串包.目标小写(引导内容), "codex") {
		测试器.格式化报告致命错误("技能目录的内置指引应描述入口格式而不含其他产品名: %q", 引导内容)
	}
	if 错误 := 操作系统包.移除(路径); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 值集, 错误 := 存储.快照(测试器.上下文()); 错误 != nil || len(值集) != 1 {
		测试器.格式化报告致命错误("删除未生效: %+v %v", 值集, 错误)
	}
}

func Test无效技能不隐藏有效技能(测试器 *测试包.测试器) {
	存储 := 新建存储(测试器)
	写入技能(测试器, 存储, "good", "---\nname: good\ndescription: valid\n---\n")
	for 名称, 内容 := range map[string]string{
		"no-header": "plain markdown",
		"duplicate": "---\nname: first\nname: duplicate\ndescription: duplicate key\n---\n",
		"missing":   "---\nname: missing\n---\n",
		"large":     字符串包.重复("x", 最大文档字节+1),
	} {
		写入技能(测试器, 存储, 名称, 内容)
	}
	if 运行时包.目标操作系统 != "windows" {
		外部 := 测试器.创建临时目录()
		if 错误 := 操作系统包.符号链接(外部, 文件路径包.合并(存储.获取目录(), "linked")); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		if 错误 := 操作系统包.写入文件(文件路径包.合并(外部, "SKILL.md"), []byte("---\nname: outside\ndescription: no\n---"), 0600); 错误 != nil {
			测试器.报告致命错误(错误)
		}
	}
	值集, 错误 := 存储.快照(测试器.上下文())
	if 错误 != nil || len(值集) != 1 || 值集[0].名称 != "good" {
		测试器.格式化报告致命错误("无效技能影响其他技能: %+v %v", 值集, 错误)
	}
	执行上下文, 取消函数 := 上下文包.创建可取消上下文(测试器.上下文())
	取消函数()
	if _, 错误 := 存储.快照(执行上下文); !错误判断包.是否属于(错误, 上下文包.已取消错误) {
		测试器.格式化报告致命错误("取消未传递: %v", 错误)
	}
}

func Test系统技能并且用户覆盖刷新(测试器 *测试包.测试器) {
	存储 := 新建存储(测试器)
	系统 := 写入技能(测试器, 存储, ".system/documents", "---\nname: documents\ndescription: bundled\n---\n")
	写入技能(测试器, 存储, ".hidden/ignored", "---\nname: hidden\ndescription: ignored\n---\n")
	值集, 错误 := 存储.快照(测试器.上下文())
	if 错误 != nil || len(值集) != 1 || 值集[0].路径 != 系统 || 值集[0].来源 != "system" {
		测试器.格式化报告致命错误("系统技能未被发现: %+v %v", 值集, 错误)
	}
	自定义 := 写入技能(测试器, 存储, "my-documents", "---\nname: documents\ndescription: customized\n---\n")
	值集, 错误 = 存储.快照(测试器.上下文())
	if 错误 != nil || len(值集) != 1 || 值集[0].路径 != 自定义 || 值集[0].来源 != "user" {
		测试器.格式化报告致命错误("用户覆盖未生效: %+v %v", 值集, 错误)
	}
	if 错误 := 操作系统包.移除(自定义); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	值集, 错误 = 存储.快照(测试器.上下文())
	if 错误 != nil || len(值集) != 1 || 值集[0].路径 != 系统 {
		测试器.格式化报告致命错误("删除覆盖后未恢复系统技能: %+v %v", 值集, 错误)
	}
}
```
