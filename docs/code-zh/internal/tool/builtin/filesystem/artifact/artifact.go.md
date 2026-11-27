```go
// 包 交付物 提供通用的本地读图和交付文件保留工具，不解析 办公文件 格式。
package 交付物

import (
	上下文包 "context"
	JSON包 "encoding/json"
	输入输出包 "io"
	文件路径包 "path/filepath"
	字符串包 "strings"

	附件包 "github.com/xiaoyu98628/callum/internal/attachment"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	图片输入包 "github.com/xiaoyu98628/callum/internal/model/imageinput"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	文件访问包 "github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/internal/fileaccess"
	参数包 "github.com/xiaoyu98628/callum/internal/tool/builtin/internal/arguments"
)

type 文件工具 struct {
	访问 *附件包.附件访问器
	保护 工具包.受保护数据
	查看 bool
}

func 工具集(访问 *附件包.附件访问器, 保护 工具包.受保护数据) []工具包.工具 {
	return []工具包.工具{文件工具{访问, 保护, true}, 文件工具{访问, 保护, false}}
}

func (当前文件工具 文件工具) 获取定义() 工具包.定义 {
	名称, 说明 := "publish_artifact", "读取本地文件并保存独立会话交付副本，在回答中显示可另存的文件卡片。本轮临时文件必须通过此工具保存后才能交付。源文件不改写；path 支持绝对路径，相对路径需要工作区。"
	if 当前文件工具.查看 {
		名称, 说明 = "view_image", "读取本地图片并作为视觉输入提供给模型，用于检查渲染页面、截图或图表。需要用户所选模型启用 image 输入。path 支持绝对路径，相对路径需要工作区。图片保存为本次工具的稳定上下文，原文件不修改。"
	}
	return 工具包.定义{名称: 名称, 说明: 说明, 参数: JSON包.原始JSON消息(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`), 来源: "builtin", 版本: "1", 影响集: []工具包.影响{工具包.影响读取}}
}

type 参数 struct {
	路径 string `json:"path"`
}

func (文件工具) 校验(原始 JSON包.原始JSON消息) (JSON包.原始JSON消息, error) {
	var 甲 参数
	if _, 错误 := 参数包.解码(原始, &甲); 错误 != nil {
		return nil, 错误
	}
	if 字符串包.裁剪空间(甲.路径) == "" || 字符串包.包含字符(甲.路径, 0) {
		return nil, 工具包.无效错误
	}
	return JSON包.序列化(甲)
}
func (当前文件工具 文件工具) 准备(执行上下文 上下文包.上下文, 范围 工具包.范围, 原始 JSON包.原始JSON消息) (工具包.工具调用, error) {
	return 当前文件工具.准备带有权限(执行上下文, 范围, 原始, 工具包.执行权限{})
}
func (当前文件工具 文件工具) 准备带有权限(执行上下文 上下文包.上下文, 范围 工具包.范围, 原始 JSON包.原始JSON消息, 权限 工具包.执行权限) (工具包.工具调用, error) {
	已规范化, 错误 := 当前文件工具.校验(原始)
	if 错误 != nil {
		return nil, 错误
	}
	var 甲 参数
	if 错误 := JSON包.反序列化(已规范化, &甲); 错误 != nil {
		return nil, 错误
	}
	if 当前文件工具.访问 == nil || 权限.执行标识 == "" || (当前文件工具.查看 && !权限.允许图片输入) {
		return nil, 工具包.不可用错误
	}
	句柄, 错误 := 文件访问包.准备(执行上下文, 范围, 甲.路径, 文件访问包.文件, 当前文件工具.保护)
	if 错误 != nil {
		return nil, 错误
	}
	return &工具调用{句柄: 句柄, 访问: 当前文件工具.访问, 权限: 权限, 查看: 当前文件工具.查看}, nil
}

type 工具调用 struct {
	*文件访问包.句柄
	访问 *附件包.附件访问器
	权限 工具包.执行权限
	查看 bool
}

func (当前工具调用 *工具调用) 执行(执行上下文 上下文包.上下文) (工具包.结果, error) {
	结果 := 工具包.结果{操作结果: 工具包.操作结果无}
	if 错误 := 当前工具调用.检查(执行上下文); 错误 != nil {
		return 结果, 错误
	}
	if 当前工具调用.获取信息().大小() > 图片输入包.最大来源字节 {
		return 结果, 工具包.超出上限错误
	}
	var 数据 []byte
	缓冲区 := make([]byte, 64<<10)
	for {
		if 错误 := 执行上下文.获取错误(); 错误 != nil {
			return 结果, 错误
		}
		数量, 错误 := 当前工具调用.获取文件().读取(缓冲区)
		数据 = append(数据, 缓冲区[:数量]...)
		if len(数据) > 图片输入包.最大来源字节 {
			return 结果, 工具包.超出上限错误
		}
		if 错误 == 输入输出包.文件结束 {
			break
		}
		if 错误 != nil {
			return 结果, 错误
		}
	}
	类别 := "file"
	if 当前工具调用.查看 {
		类别 = "image"
	}
	内容块, 错误 := 当前工具调用.访问.保存已生成(执行上下文, 当前工具调用.权限.会话标识, 当前工具调用.权限.执行标识, 类别, 文件路径包.基准(当前工具调用.获取目标().路径), 数据, !当前工具调用.查看)
	if 错误 != nil {
		return 结果, 错误
	}
	结果.输出, 错误 = JSON包.序列化(map[string]any{"attachment": 内容块, "published": !当前工具调用.查看})
	if 当前工具调用.查看 {
		结果.图片集 = []模型包.内容块{内容块}
	}
	return 结果, 错误
}
```
