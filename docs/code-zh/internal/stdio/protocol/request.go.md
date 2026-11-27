```go
package 协议

import (
	字节包 "bytes"
	JSON包 "encoding/json"
	输入输出包 "io"
	UTF8包 "unicode/utf8"
)

// 请求 是解析后的单个 JSON-RPC 请求；标识 为 nil 表示通知。
type 请求 struct {
	JSONRPC string         `json:"jsonrpc"`
	标识      JSON包.原始JSON消息 `json:"id"`
	方法      string         `json:"method"`
	参数      JSON包.原始JSON消息 `json:"params"`
}

// 解析请求 校验逐行 JSON-RPC 请求，不接受批量请求。
func 解析请求(行 []byte) (请求, *错误) {
	var 请求 请求
	if !UTF8包.有效(行) || !JSON包.有效(行) {
		return 请求, &错误{代码: -32700, 消息: "JSON 格式无效"}
	}
	已裁剪 := 字节包.裁剪空间(行)
	// 本协议按行传递单个请求或通知对象，批量数组不属于支持的通信格式。
	var 字段集 map[string]JSON包.原始JSON消息
	if len(已裁剪) == 0 || 已裁剪[0] != '{' || JSON包.反序列化(行, &字段集) != nil ||
		JSON包.反序列化(字段集["jsonrpc"], &请求.JSONRPC) != nil || JSON包.反序列化(字段集["method"], &请求.方法) != nil || 请求.JSONRPC != "2.0" || 请求.方法 == "" {
		return 请求{}, &错误{代码: -32600, 消息: "请求对象无效"}
	}
	请求.标识, 请求.参数 = 字段集["id"], 字段集["params"]
	if 请求.标识 != nil {
		var 标识 any
		解码器 := JSON包.新建解码器(字节包.新建读取器(请求.标识))
		解码器.使用数量()
		if 解码器.解码(&标识) != nil {
			return 请求{}, &错误{代码: -32600, 消息: "请求 ID 无效"}
		}
		switch 标识.(type) {
		case string, JSON包.数量, nil:
		default:
			return 请求{}, &错误{代码: -32600, 消息: "请求 ID 无效"}
		}
	}
	return 请求, nil
}

// 解码参数 解析具名参数对象，拒绝未知字段和无效类型。
func 解码参数(原始 JSON包.原始JSON消息, 目标 any) *错误 {
	if 原始 == nil {
		原始 = JSON包.原始JSON消息(`{}`)
	}
	if 已裁剪 := 字节包.裁剪空间(原始); len(已裁剪) == 0 || 已裁剪[0] != '{' {
		return 无效参数()
	}
	解码器 := JSON包.新建解码器(字节包.新建读取器(原始))
	解码器.禁止未知字段集()
	if 解码器.解码(目标) != nil {
		return 无效参数()
	}
	if 解码器.解码(new(any)) != 输入输出包.文件结束 {
		return 无效参数()
	}
	return nil
}
```
