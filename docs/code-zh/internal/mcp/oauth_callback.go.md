```go
package MCP

import (
	字节包 "bytes"
	_ "embed"
	基准64包 "encoding/base64"
	模板包 "html/template"
	HTTP包 "net/http"
)

//go:embed oauth_callback.html
var OAuth回调HTML string

var OAuth回调模板 = 模板包.必须(模板包.新建("oauth_callback").解析(OAuth回调HTML))

type OAuth回调页 struct {
	状态  string
	标题  string
	服务  string
	消息  string
	提示  string
	随机数 string
}

func 写入OAuth回调页(当前响应写入器 HTTP包.响应写入器, 代码 int, 服务 string, 状态 OAuth状态) {
	页 := OAuth回调页{
		状态: 状态.状态, 标题: "授权失败", 服务: 服务, 消息: 状态.消息,
		提示:  "可以关闭此标签页，回到 Callum 后重试。",
		随机数: 基准64包.原始URL编码.编码目标字符串(随机字节()),
	}
	switch 状态.状态 {
	case "authorized":
		页.标题 = "授权成功"
		页.提示 = "可以关闭此标签页，返回 Callum 测试连接。"
	case "cancelled":
		页.标题 = "已取消授权"
		if 页.消息 == "" {
			页.消息 = "这次授权已取消。"
		}
		页.提示 = "可以关闭此标签页，返回 Callum。"
	}
	var 正文 字节包.缓冲区
	if 错误 := OAuth回调模板.执行(&正文, 页); 错误 != nil {
		HTTP包.错误文本(当前响应写入器, "无法显示授权结果，请返回 Callum。", HTTP包.状态内部服务端错误)
		return
	}
	当前响应写入器.头部().设置("Content-Type", "text/html; charset=utf-8")
	当前响应写入器.头部().设置("X-Content-Type-Options", "nosniff")
	当前响应写入器.头部().设置("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'nonce-"+页.随机数+"'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	当前响应写入器.写入头部(代码)
	// 浏览器关闭或网络写入失败不影响已经保存的授权结果。
	_, _ = 正文.写入目标(当前响应写入器)
}
```
