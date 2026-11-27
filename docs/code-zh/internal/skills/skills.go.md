```go
// 包 技能 发现应用数据目录中以 技能.md 声明的技能，不执行扫描到的脚本。
package 技能

import (
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"
	格式化包 "fmt"
	输入输出包 "io"
	结构化日志包 "log/slog"
	操作系统包 "os"
	文件路径包 "path/filepath"
	运行时包 "runtime"
	切片包 "slices"
	字符串包 "strings"
	Unicode包 "unicode"
	UTF8包 "unicode/utf8"

	YAML包 "go.yaml.in/yaml/v3"
)

const 最大文档字节 = 64 << 10
const 最大条目集 = 4096
const 最大目录管理字节 = 64 << 10

// 存储 只保存目录身份；每次 快照 重新发现新增、修改及删除的技能。
// 不持有文件、监视器或 goroutine，调用方无需关闭。
type 存储 struct {
	目录 string
	身份  操作系统包.文件信息
	日志记录器    *结构化日志包.日志记录器
}

// 条目 是技能目录的元数据。路径 始终指向当前技能的实际 技能.md。
type 条目 struct {
	名称         string `json:"name"`
	说明  string `json:"description"`
	路径         string `json:"path"`
	来源       string `json:"source"`
	显式仅 bool   `json:"explicit_only,omitempty"`
}

// 打开 在已取得应用实例锁后创建 技能 目录，不覆盖用户已有文件。
func 打开(执行上下文 上下文包.上下文, 数据目录 string, 日志记录器 *结构化日志包.日志记录器) (_ *存储, 返回错误 error) {
	if !文件路径包.是否绝对路径(数据目录) || 日志记录器 == nil {
		return nil, 错误判断包.新建("技能目录配置无效")
	}
	目录 := 文件路径包.合并(数据目录, "skills")
	if 错误 := 操作系统包.创建目录(目录, 0700); 错误 != nil && !错误判断包.是否属于(错误, 操作系统包.存在错误) {
		return nil, 错误
	}
	信息, 错误 := 操作系统包.读取链接自身状态(目录)
	if 错误 != nil || !信息.是否目录() || 信息.模式()&操作系统包.模式符号链接 != 0 || (运行时包.目标操作系统 != "windows" && 信息.模式().权限()&0077 != 0) {
		return nil, 错误判断包.新建("技能目录必须是私有普通目录")
	}
	根, 错误 := 操作系统包.打开根(目录)
	if 错误 != nil {
		return nil, 错误
	}
	defer func() { 返回错误 = 错误判断包.合并(返回错误, 根.关闭()) }()
	已打开, 错误 := 根.状态信息(".")
	if 错误 != nil || !操作系统包.相同文件(信息, 已打开) {
		return nil, 错误判断包.新建("技能根目录身份已变化")
	}
	if 错误 := 执行上下文.获取错误(); 错误 != nil {
		return nil, 错误
	}
	return &存储{目录: 目录, 身份: 信息, 日志记录器: 日志记录器}, nil
}

// 目录 返回与 附件集 同级、已经固定身份的技能根目录。
func (当前存储 *存储) 获取目录() string { return 当前存储.目录 }

// 快照 重新读取技能元数据；坏文件仅隔离该技能并记录诊断，不执行任意入口。
// 扫描、单文件和目录上下文都有独立资源边界，取消沿调用方传播。
func (当前存储 *存储) 快照(执行上下文 上下文包.上下文) (_ []条目, 返回错误 error) {
	信息, 错误 := 操作系统包.读取链接自身状态(当前存储.目录)
	if 错误 != nil || !信息.是否目录() || !操作系统包.相同文件(信息, 当前存储.身份) {
		return nil, 错误判断包.新建("技能根目录身份已变化")
	}
	根, 错误 := 操作系统包.打开根(当前存储.目录)
	if 错误 != nil {
		return nil, 错误
	}
	defer func() { 返回错误 = 错误判断包.合并(返回错误, 根.关闭()) }()
	已打开, 错误 := 根.状态信息(".")
	if 错误 != nil || !操作系统包.相同文件(已打开, 当前存储.身份) {
		return nil, 错误判断包.新建("技能根目录身份已变化")
	}
	var 条目集 []条目
	已扫描 := 0
	var 遍历 func(string, int) error
	遍历 = func(目录 string, 深度 int) (返回错误 error) {
		if 错误 := 执行上下文.获取错误(); 错误 != nil {
			return 错误
		}
		if 深度 > 8 {
			return 错误判断包.新建("技能目录嵌套超过限制")
		}
		if 目录 != "." && 目录 != ".system" {
			名称 := 文件路径包.合并(目录, "SKILL.md")
			if 信息, 错误 := 根.读取链接自身状态(名称); 错误 == nil {
				if !信息.模式().是否普通文件() || 信息.大小() > 最大文档字节 {
					当前存储.警告(名称, "技能入口不是有界普通文件")
					return nil
				}
				数据, 错误 := 读取文档(根, 名称)
				if 错误 != nil {
					当前存储.警告(名称, "无法读取技能入口")
					return nil
				}
				条目, 错误 := 解析(数据)
				if 错误 != nil {
					当前存储.警告(名称, 错误.错误文本())
					return nil
				}
				条目.路径 = 文件路径包.合并(当前存储.目录, 名称)
				条目.来源 = "user"
				if 字符串包.存在前缀(文件路径包.目标斜杠(目录), ".system/") {
					条目.来源 = "system"
				}
				策略, 策略错误 := 读取文档(根, 文件路径包.合并(目录, "agents", "callum.yaml"))
				if 策略错误 == nil {
					var 元数据 struct {
						策略 struct {
							允许隐式工具调用 *bool `yaml:"allow_implicit_invocation"`
						} `yaml:"policy"`
					}
					if 错误 := YAML包.反序列化(策略, &元数据); 错误 != nil {
						当前存储.警告(名称, "agents/callum.yaml 无效")
						return nil
					}
					条目.显式仅 = 元数据.策略.允许隐式工具调用 != nil && !*元数据.策略.允许隐式工具调用
				} else if !错误判断包.是否属于(策略错误, 操作系统包.不存在错误) {
					当前存储.警告(名称, "无法读取技能策略")
					return nil
				}
				条目集 = append(条目集, 条目)
				return nil
			} else if !错误判断包.是否属于(错误, 操作系统包.不存在错误) {
				return 错误
			}
		}
		文件, 错误 := 根.打开(目录)
		if 错误 != nil {
			return 错误
		}
		defer func() { 返回错误 = 错误判断包.合并(返回错误, 文件.关闭()) }()
		for {
			子项集, 错误 := 文件.读取目录(128)
			if 错误 != nil && 错误 != 输入输出包.文件结束 {
				return 错误
			}
			for _, 子级 := range 子项集 {
				已扫描++
				if 已扫描 > 最大条目集 {
					return 错误判断包.新建("技能目录条目超过限制")
				}
				if 子级.是否目录() && (!字符串包.存在前缀(子级.名称(), ".") || (目录 == "." && 子级.名称() == ".system")) && 子级.名称() != "node_modules" {
					if 错误 := 遍历(文件路径包.合并(目录, 子级.名称()), 深度+1); 错误 != nil {
						return 错误
					}
				}
			}
			if 错误 == 输入输出包.文件结束 {
				return nil
			}
		}
	}
	if 错误 := 遍历(".", 0); 错误 != nil {
		return nil, 错误
	}
	切片包.排序函数(条目集, func(当前条目, 当前条目 条目) int { return 字符串包.比较(当前条目.路径, 当前条目.路径) })
	// 用户技能按元数据 名称 覆盖同名系统技能；同来源的重名保留实际路径。
	用户名称集 := make(map[string]bool)
	for _, 条目 := range 条目集 {
		if 条目.来源 == "user" {
			用户名称集[条目.名称] = true
		}
	}
	条目集 = 切片包.删除函数(条目集, func(条目 条目) bool { return 条目.来源 == "system" && 用户名称集[条目.名称] })
	return 条目集, 执行上下文.获取错误()
}

func 读取文档(根 *操作系统包.根, 名称 string) (_ []byte, 返回错误 error) {
	信息, 错误 := 根.读取链接自身状态(名称)
	if 错误 != nil {
		return nil, 错误
	}
	if !信息.模式().是否普通文件() || 信息.大小() > 最大文档字节 {
		return nil, 错误判断包.新建("技能元数据文件类型或大小无效")
	}
	文件, 错误 := 根.打开文件(名称, 读取参数集, 0)
	if 错误 != nil {
		return nil, 错误
	}
	defer func() { 返回错误 = 错误判断包.合并(返回错误, 文件.关闭()) }()
	已打开, 错误 := 文件.状态信息()
	if 错误 != nil || !操作系统包.相同文件(信息, 已打开) {
		return nil, 错误判断包.新建("技能入口身份已变化")
	}
	数据, 错误 := 输入输出包.读取全部(输入输出包.上限读取器(文件, 最大文档字节+1))
	if 错误 != nil || len(数据) > 最大文档字节 {
		return nil, 错误判断包.新建("技能入口读取失败或超限")
	}
	return 数据, nil
}

func 解析(数据 []byte) (条目, error) {
	文本 := 字符串包.裁剪前缀(string(数据), "\ufeff")
	文本 = 字符串包.替换全部(文本, "\r\n", "\n")
	if !UTF8包.有效字符串(文本) || !字符串包.存在前缀(文本, "---\n") {
		return 条目{}, 错误判断包.新建("缺少 YAML frontmatter")
	}
	行集 := 字符串包.拆分(文本, "\n")
	结束 := -1
	for 索引 := 1; 索引 < len(行集); 索引++ {
		if 字符串包.裁剪空间(行集[索引]) == "---" {
			结束 = 索引
			break
		}
	}
	if 结束 < 0 {
		return 条目{}, 错误判断包.新建("技能元数据未结束")
	}
	var 元数据 struct {
		名称        string `yaml:"name"`
		说明 string `yaml:"description"`
	}
	if 错误 := YAML包.反序列化([]byte(字符串包.合并(行集[1:结束], "\n")), &元数据); 错误 != nil {
		return 条目{}, 错误判断包.新建("技能 YAML 元数据无效")
	}
	名称, 说明 := 字符串包.裁剪空间(元数据.名称), 字符串包.合并(字符串包.字段集(元数据.说明), " ")
	if 名称 == "" || len(名称) > 128 || 说明 == "" || len(说明) > 4096 || 字符串包.包含函数(名称, Unicode包.是控制) {
		return 条目{}, 错误判断包.新建("技能 name 或 description 无效")
	}
	return 条目{名称: 名称, 说明: 说明}, nil
}

func (当前存储 *存储) 警告(路径, 原因 string) {
	当前存储.日志记录器.警告("忽略无效技能", "module", "skills", "path", 路径, "reason", 原因)
}

// 目录管理 返回供模型和客户端共用的有界目录；没有技能时返回空 JSON 数组。
func (当前存储 *存储) 目录管理(执行上下文 上下文包.上下文) (JSON包.原始JSON消息, error) {
	条目集, 错误 := 当前存储.快照(执行上下文)
	if 错误 != nil {
		return nil, 错误
	}
	if len(条目集) == 0 {
		return JSON包.原始JSON消息("[]"), nil
	}
	数据, 错误 := JSON包.序列化(条目集)
	if 错误 != nil {
		return nil, 错误
	}
	if len(数据) > 最大目录管理字节 {
		return nil, 格式化包.格式化创建错误("技能目录上下文超过 %d 字节", 最大目录管理字节)
	}
	return 数据, nil
}

// 上下文引导内容 复用上游技能选择规则，并说明本服务实际提供的工具和目录字段。
const 上下文引导内容 = `# Using skills

A skill is a set of instructions provided through a SKILL.md source.

The user's instructions take precedence over guidelines provided in a skill. If explicit user instructions conflict with a skill's instructions, prioritize the user's instructions.

The first time in a conversation that you decide to apply a skill, inform the user in your progress update.

## When to use a skill

If the user names a skill (with $SkillName or plain text), use that skill. If the file is missing, check the supplied catalog for the matching name and source. If a required skill is missing or cannot be loaded, explain why and continue with work that can be done within scope.

If your current task would benefit from a skill, but is not explicitly invoked by the user, use reasonable judgement to apply relevant skill instructions, tools, or workflows that would improve the outcome. Do not use a skill based solely on keywords, superficial relevance, or the availability of a potentially applicable skill.

## How to use skills

Open the skill's SKILL.md with read_file using its path from the catalog. Read referenced resources as needed and resolve relative references from the SKILL.md directory. Avoid re-reading skills when possible.

Use explicit_only skills only when the user names them. A user skill takes precedence over a system skill of the same name. Use the actual path to distinguish repeated names from the same source. Skills do not grant tool permissions. Use open_attachment to obtain a per-run copy before passing a regular attachment to a skill script.

## Available skills / 可用技能

The JSON entries below list each skill's name, description and SKILL.md path. The source field identifies system and user skills.`

// 上下文 将技能目录加入模型上下文；技能内容不能改变权限规则。
func (当前存储 *存储) 上下文(执行上下文 上下文包.上下文) (string, error) {
	数据, 错误 := 当前存储.目录管理(执行上下文)
	if 错误 != nil || len(数据) == len("[]") {
		return "", 错误
	}
	return 上下文引导内容 + "\n" + string(数据), nil
}
```
