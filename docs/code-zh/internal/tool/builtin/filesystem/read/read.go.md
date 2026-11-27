```go
// 包 读取 提供固定目标范围的按行文本读取工具。
package 读取

import (
	上下文包 "context"
	SHA256包 "crypto/sha256"
	十六进制包 "encoding/hex"
	JSON包 "encoding/json"
	字符串包 "strings"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	文件访问包 "github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/internal/fileaccess"
)

type 读取工具 struct{ 保护 工具包.受保护数据 }

// 新建 返回独立的按行文本读取工具。
func 新建(保护 工具包.受保护数据) 工具包.工具 { return 读取工具{保护} }

func (读取工具) 获取定义() 工具包.定义 {
	return 工具包.定义{名称: "read_file", 说明: "按行读取 UTF-8 文本文件。path 支持绝对路径；相对路径需要绑定工作区。只读访问不受工作区写入边界限制，审批由服务端策略判断", 参数: JSON包.原始JSON消息(`{"type":"object","properties":{"path":{"type":"string"},"start_line":{"type":"integer","minimum":1},"end_line":{"type":"integer","minimum":1}},"required":["path"],"additionalProperties":false}`), 来源: "builtin", 版本: "3", 影响集: []工具包.影响{工具包.影响读取}}
}

func (读取工具) 校验(原始 JSON包.原始JSON消息) (JSON包.原始JSON消息, error) {
	甲, 错误 := 解码参数(原始)
	if 错误 != nil {
		return nil, 错误
	}
	return JSON包.序列化(甲)
}

func (当前读取工具 读取工具) 准备(执行上下文 上下文包.上下文, 范围 工具包.范围, 原始 JSON包.原始JSON消息) (工具包.工具调用, error) {
	甲, 错误 := 解码参数(原始)
	if 错误 != nil {
		return nil, 错误
	}
	句柄, 错误 := 文件访问包.准备(执行上下文, 范围, 甲.路径, 文件访问包.文件, 当前读取工具.保护)
	if 错误 != nil {
		return nil, 错误
	}
	return &工具调用{句柄: 句柄, 参数: 甲}, nil
}

type 工具调用 struct {
	*文件访问包.句柄
	参数 参数
}

func (当前工具调用 *工具调用) 执行(执行上下文 上下文包.上下文) (工具包.结果, error) {
	if 错误 := 当前工具调用.检查(执行上下文); 错误 != nil {
		return 工具包.结果{操作结果: 工具包.操作结果无}, 错误
	}
	数据, 错误 := 文件访问包.读取文本(执行上下文, 当前工具调用.获取文件())
	if 错误 != nil {
		return 工具包.结果{操作结果: 工具包.操作结果无}, 错误
	}
	甲 := 当前工具调用.参数
	甲.路径 = 当前工具调用.获取目标().路径
	输出, 错误 := 读取(甲, 数据)
	return 工具包.结果{输出: 输出, 操作结果: 工具包.操作结果无}, 错误
}

func 读取(当前参数 参数, 数据 []byte) (JSON包.原始JSON消息, error) {
	行集 := 字符串包.拆分(string(数据), "\n")
	启动 := min(当前参数.启动-1, len(行集))
	结束 := min(当前参数.结束, len(行集))
	文本 := 字符串包.合并(行集[启动:结束], "\n")
	if len(文本) > 工具包.最大结果字节-4096 {
		return nil, 工具包.超出上限错误
	}
	校验和 := SHA256包.校验和256(数据)
	return JSON包.序列化(struct {
		路径     string `json:"path"`
		启动     int    `json:"start_line"`
		结束     int    `json:"end_line"`
		文本     string `json:"text"`
		已截取    bool   `json:"truncated"`
		SHA256 string `json:"sha256"`
	}{当前参数.路径, 当前参数.启动, 结束, 文本, 结束 < len(行集), 十六进制包.编码目标字符串(校验和[:])})
}
```
