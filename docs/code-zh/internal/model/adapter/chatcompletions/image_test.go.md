```go
package 聊天补全

import (
	字节包 "bytes"
	上下文包 "context"
	基准64包 "encoding/base64"
	JSON包 "encoding/json"
	图片包 "image"
	PNG包 "image/png"
	输入输出包 "io"
	HTTP包 "net/http"
	字符串包 "strings"
	测试包 "testing"

	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

func Test图片准备已复用依据执行前检查并且发送(测试器 *测试包.测试器) {
	var 数据 字节包.缓冲区
	if 错误 := PNG包.编码(&数据, 图片包.新建RGBA(图片包.矩形(0, 0, 1, 1))); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	通信图片 := "data:image/png;base64," + 基准64包.标准编码.编码目标字符串(数据.字节())
	读取 := 0
	客户端 := 新建测试客户端(测试器, func(请求 *HTTP包.请求) (*HTTP包.响应, error) {
		通信, 错误 := 输入输出包.读取全部(请求.正文)
		if 错误 != nil || !字节包.包含(通信, []byte(通信图片)) {
			测试器.格式化报告致命错误("实际发送的数据不一致: %v", 错误)
		}
		return &HTTP包.响应{状态代码: 200, 头部: HTTP包.头部{"Content-Type": {"application/json"}},
			正文: 输入输出包.空操作关闭器(字符串包.新建读取器(`{"choices":[{"index":0,"message":{"role":"assistant","content":"已收到"},"finish_reason":"stop"}]}`))}, nil
	})
	客户端.读取附件 = func(上下文包.上下文, string) ([]byte, string, error) {
		读取++
		return 数据.字节(), "image/png", nil
	}
	请求 := 模型包.请求{模型: "vision", 允许图片输入: true, 消息集: []模型包.消息{{
		角色: 模型包.角色用户, 内容块集: []模型包.内容块{{类型: "image", 附件引用: "managed"}},
	}}}
	if 错误 := 客户端.校验输入(测试器.上下文(), 请求); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if _, 错误 := 客户端.生成(测试器.上下文(), 请求); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 读取 != 1 {
		测试器.格式化报告致命错误("预检与发送重复读取图片: %d", 读取)
	}
}

func Test工具图片集后续完成工具批次并且保留文本模型切换(测试器 *测试包.测试器) {
	var 数据 字节包.缓冲区
	if 错误 := PNG包.编码(&数据, 图片包.新建RGBA(图片包.矩形(0, 0, 1, 1))); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	客户端 := 新建测试客户端(测试器, nil)
	客户端.读取附件 = func(上下文包.上下文, string) ([]byte, string, error) { return 数据.字节(), "image/png", nil }
	请求 := 模型包.请求{模型: "vision", 允许图片输入: true, 消息集: []模型包.消息{
		{角色: 模型包.角色助手, 工具调用集: []模型包.工具调用{{标识: "first", 名称: "view_image", 参数: JSON包.原始JSON消息(`{}`)}, {标识: "second", 名称: "read_file", 参数: JSON包.原始JSON消息(`{}`)}}},
		{角色: 模型包.角色工具, 工具调用标识: "first", 内容块集: []模型包.内容块{{类型: "text", 文本: "first tool result"}, {类型: "image", 附件引用: "managed"}}},
		{角色: 模型包.角色工具, 工具调用标识: "second", 内容: "second tool result"},
		{角色: 模型包.角色助手, 内容: "review completed"},
	}}
	for _, 视觉 := range []bool{true, false} {
		请求.允许图片输入 = 视觉
		参数, 错误 := 客户端.构建请求(测试器.上下文(), 请求)
		if 错误 != nil {
			测试器.报告致命错误(错误)
		}
		已编码, 错误 := JSON包.序列化(参数)
		if 错误 != nil {
			测试器.报告致命错误(错误)
		}
		var 通信 struct {
			消息集 []struct {
				角色 string         `json:"role"`
				内容 JSON包.原始JSON消息 `json:"content"`
			} `json:"messages"`
		}
		if 错误 := JSON包.反序列化(已编码, &通信); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		if 视觉 {
			if len(通信.消息集) != 5 || 通信.消息集[1].角色 != "tool" || 通信.消息集[2].角色 != "tool" || 通信.消息集[3].角色 != "user" || !字节包.包含(通信.消息集[3].内容, []byte("data:image/png;base64,")) || 字节包.包含(通信.消息集[3].内容, []byte("first tool result")) {
				测试器.格式化报告致命错误("视觉内容破坏工具配对或重复正文: %s", 已编码)
			}
		} else if len(通信.消息集) != 4 || 字节包.包含(已编码, []byte("image_url")) || !字节包.包含(已编码, []byte("未提供给当前文字模型")) {
			测试器.格式化报告致命错误("文字模型切换没有明确省略历史工具图片: %s", 已编码)
		}
	}
}
```
