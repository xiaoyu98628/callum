```go
// 包 搜索 提供固定目标范围的字面文本搜索工具。
package 搜索

import (
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"
	输入输出包 "io"
	操作系统包 "os"
	文件路径包 "path/filepath"
	字符串包 "strings"
	UTF8包 "unicode/utf8"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	文件访问包 "github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/internal/fileaccess"
)

type 搜索工具 struct{ 保护 工具包.受保护数据 }

// 新建 返回独立的字面文本搜索工具。
func 新建(保护 工具包.受保护数据) 工具包.工具 { return 搜索工具{保护} }

func (搜索工具) 获取定义() 工具包.定义 {
	return 工具包.定义{名称: "search_text", 说明: "在目录或文件中搜索字面文本。path 支持绝对路径；相对路径需要绑定工作区。只读访问不受工作区写入边界限制，审批由服务端策略判断", 参数: JSON包.原始JSON消息(`{"type":"object","properties":{"path":{"type":"string"},"query":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":100}},"required":["path","query"],"additionalProperties":false}`), 来源: "builtin", 版本: "3", 影响集: []工具包.影响{工具包.影响读取}}
}

func (搜索工具) 校验(原始 JSON包.原始JSON消息) (JSON包.原始JSON消息, error) {
	甲, 错误 := 解码参数(原始)
	if 错误 != nil {
		return nil, 错误
	}
	return JSON包.序列化(甲)
}

func (当前搜索工具 搜索工具) 准备(执行上下文 上下文包.上下文, 范围 工具包.范围, 原始 JSON包.原始JSON消息) (工具包.工具调用, error) {
	甲, 错误 := 解码参数(原始)
	if 错误 != nil {
		return nil, 错误
	}
	句柄, 错误 := 文件访问包.准备(执行上下文, 范围, 甲.路径, 文件访问包.文件或者目录, 当前搜索工具.保护)
	if 错误 != nil {
		return nil, 错误
	}
	return &工具调用{句柄: 句柄, 参数: 甲, 保护: 当前搜索工具.保护}, nil
}

type 工具调用 struct {
	*文件访问包.句柄
	参数 参数
	保护 工具包.受保护数据
}

func (当前工具调用 *工具调用) 执行(执行上下文 上下文包.上下文) (工具包.结果, error) {
	if 错误 := 当前工具调用.检查(执行上下文); 错误 != nil {
		return 工具包.结果{操作结果: 工具包.操作结果无}, 错误
	}
	甲 := 当前工具调用.参数
	甲.路径 = "."
	输出, 错误 := 搜索(执行上下文, 当前工具调用.获取根(), 当前工具调用.获取文件(), 甲, 当前工具调用.获取目标().路径, 当前工具调用.保护)
	return 工具包.结果{输出: 输出, 操作结果: 工具包.操作结果无}, 错误
}

type 匹配 struct {
	路径 string `json:"path"`
	行  int    `json:"line"`
	文本 string `json:"text"`
}

func 搜索(执行上下文 上下文包.上下文, 根 *操作系统包.根, 文件 *操作系统包.文件, 当前参数 参数, 基准 string, 保护 工具包.受保护数据) (JSON包.原始JSON消息, error) {
	输出 := make([]匹配, 0)
	已访问, 字节读取 := 0, 0
	已截取 := false
	var 遍历 func(*操作系统包.根, string, string, int) error
	遍历 = func(父级 *操作系统包.根, 名称, 路径 string, 深度 int) (返回错误 error) {
		if 错误 := 执行上下文.获取错误(); 错误 != nil {
			return 错误
		}
		if 深度 > 32 || 已访问 >= 1000 || 字节读取 >= 8<<20 || len(输出) >= 当前参数.上限 {
			已截取 = true
			return nil
		}
		if 保护.读取受保护(文件路径包.合并(基准, 路径)) {
			return nil
		}
		已访问++
		var 状态信息 操作系统包.文件信息
		var 错误 error
		if 文件 != nil {
			状态信息, 错误 = 文件.状态信息()
		} else {
			状态信息, 错误 = 父级.读取链接自身状态(名称)
		}
		if 错误 != nil {
			return 工具包.已拒绝错误
		}
		// 目录搜索不跟随子项符号链接，不扩大本次固定目录的访问范围。
		if 状态信息.模式()&操作系统包.模式符号链接 != 0 {
			return nil
		}
		if 状态信息.是否目录() {
			// 每层使用已打开的父目录句柄，避免递归时重新经过可被替换的祖先路径。
			子级, 错误 := 父级.打开根(名称)
			if 错误 != nil {
				return 工具包.已拒绝错误
			}
			defer func() { 返回错误 = 错误判断包.合并(返回错误, 子级.关闭()) }()
			文件, 错误 := 文件访问包.打开读取(子级, ".", 状态信息)
			if 错误 != nil {
				return 错误
			}
			条目集, 读取错误 := 文件.读取目录(1001)
			关闭错误 := 文件.关闭()
			// 空目录返回 文件结束，属于正常遍历结果；关闭错误仍需保留。
			if 错误判断包.是否属于(读取错误, 输入输出包.文件结束) {
				读取错误 = nil
			}
			if 错误 := 错误判断包.合并(读取错误, 关闭错误); 错误 != nil {
				return 错误
			}
			for _, 条目 := range 条目集 {
				if 已访问 >= 1000 || len(输出) >= 当前参数.上限 || 字节读取 >= 8<<20 {
					已截取 = true
					break
				}
				if 条目.名称() == ".git" {
					continue
				}
				if 错误 := 遍历(子级, 条目.名称(), 文件路径包.合并(路径, 条目.名称()), 深度+1); 错误 != nil {
					return 错误
				}
			}
			if len(条目集) > 1000 {
				已截取 = true
			}
			return nil
		}
		if !状态信息.模式().是否普通文件() {
			return nil
		}
		var 数据 []byte
		if 文件 != nil {
			数据, 错误 = 文件访问包.读取文本(执行上下文, 文件)
		} else {
			数据, 错误 = 文件访问包.读取文本文件(执行上下文, 父级, 名称, 状态信息, 保护)
		}
		if 错误判断包.是否属于(错误, 工具包.超出上限错误) || 错误判断包.是否属于(错误, 工具包.不可用错误) {
			已截取 = true
			return nil
		}
		if 错误 != nil {
			return 错误
		}
		字节读取 += len(数据)
		for 索引, 行 := range 字符串包.拆分(string(数据), "\n") {
			if 字符串包.包含(行, 当前参数.查询) {
				if len(行) > 1024 {
					行 = 行[:1024]
					for len(行) > 0 && !UTF8包.有效字符串(行) {
						行 = 行[:len(行)-1]
					}
					已截取 = true
				}
				输出 = append(输出, 匹配{文件路径包.合并(基准, 路径), 索引 + 1, 行})
				if len(输出) >= 当前参数.上限 {
					已截取 = true
					break
				}
			}
		}
		return nil
	}
	if 错误 := 遍历(根, 当前参数.路径, 当前参数.路径, 0); 错误 != nil {
		return nil, 错误
	}
	return JSON包.序列化(struct {
		匹配集 []匹配 `json:"matches"`
		已截取 bool `json:"truncated"`
	}{输出, 已截取})
}
```
