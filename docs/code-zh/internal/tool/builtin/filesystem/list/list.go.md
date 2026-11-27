```go
// 包 列表 提供固定目标范围的目录查看工具。
package 列表

import (
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"
	输入输出包 "io"
	操作系统包 "os"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	文件访问包 "github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/internal/fileaccess"
)

type 目录工具 struct{ 保护 工具包.受保护数据 }

// 新建 返回独立的目录查看工具。
func 新建(保护 工具包.受保护数据) 工具包.工具 { return 目录工具{保护} }

func (目录工具) 获取定义() 工具包.定义 {
	return 工具包.定义{名称: "list_directory", 说明: "查看目录中的文件和子目录。path 支持绝对路径；相对路径需要绑定工作区。只读访问不受工作区写入边界限制，审批由服务端策略判断", 参数: JSON包.原始JSON消息(`{"type":"object","properties":{"path":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":200}},"required":["path"],"additionalProperties":false}`), 来源: "builtin", 版本: "3", 影响集: []工具包.影响{工具包.影响读取}}
}

func (目录工具) 校验(原始 JSON包.原始JSON消息) (JSON包.原始JSON消息, error) {
	甲, 错误 := 解码参数(原始)
	if 错误 != nil {
		return nil, 错误
	}
	return JSON包.序列化(甲)
}

func (当前目录工具 目录工具) 准备(执行上下文 上下文包.上下文, 范围 工具包.范围, 原始 JSON包.原始JSON消息) (工具包.工具调用, error) {
	甲, 错误 := 解码参数(原始)
	if 错误 != nil {
		return nil, 错误
	}
	句柄, 错误 := 文件访问包.准备(执行上下文, 范围, 甲.路径, 文件访问包.目录, 当前目录工具.保护)
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
	甲 := 当前工具调用.参数
	甲.路径 = "."
	输出, 错误 := 列表(执行上下文, 当前工具调用.获取根(), 甲)
	return 工具包.结果{输出: 输出, 操作结果: 工具包.操作结果无}, 错误
}

func 列表(执行上下文 上下文包.上下文, 根 *操作系统包.根, 当前参数 参数) (_ JSON包.原始JSON消息, 返回错误 error) {
	文件, 错误 := 文件访问包.打开读取(根, 当前参数.路径, nil)
	if 错误 != nil {
		return nil, 错误
	}
	defer func() { 返回错误 = 错误判断包.合并(返回错误, 文件.关闭()) }()
	if 错误 := 执行上下文.获取错误(); 错误 != nil {
		return nil, 错误
	}
	条目集, 错误 := 文件.读取目录(当前参数.上限 + 1)
	if 错误 != nil && !错误判断包.是否属于(错误, 输入输出包.文件结束) {
		return nil, 错误
	}
	已截取 := len(条目集) > 当前参数.上限
	if 已截取 {
		条目集 = 条目集[:当前参数.上限]
	}
	type 条目 struct {
		名称   string `json:"name"`
		目录   bool   `json:"directory"`
		符号链接 bool   `json:"symlink"`
	}
	输出 := make([]条目, 0, len(条目集))
	for _, 当前条目 := range 条目集 {
		if 错误 := 执行上下文.获取错误(); 错误 != nil {
			return nil, 错误
		}
		输出 = append(输出, 条目{当前条目.名称(), 当前条目.是否目录(), 当前条目.类型()&操作系统包.模式符号链接 != 0})
	}
	return JSON包.序列化(struct {
		条目集 []条目 `json:"entries"`
		已截取 bool `json:"truncated"`
	}{输出, 已截取})
}
```
