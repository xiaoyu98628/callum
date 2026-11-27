```go
package 执行

import (
	推理调用包 "github.com/xiaoyu98628/callum/internal/inference"
	协议包 "github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

// 记录错误 为对话通信边界复用模型调用的受控错误转换，不暴露底层错误链。
func 错误(错误 error) *协议包.错误 {
	if 错误 == nil {
		return nil
	}
	return 公开错误(错误)
}

func 公开错误(错误 error) *协议包.错误 { return 协议错误(推理调用包.描述错误(错误)) }

// 协议错误 仅在协议边界分配 JSON-RPC 错误码。
func 协议错误(当前失败 推理调用包.失败) *协议包.错误 {
	if 当前失败.类别 == "invalid_params" {
		return 协议包.无效参数()
	}
	代码 := 1009
	switch 当前失败.类别 {
	case "internal":
		代码 = -32603
	case "not_found":
		代码 = 1001
	case "conflict":
		代码 = 1002
	case "referenced":
		代码 = 1003
	case "disabled":
		代码 = 1004
	case "unsupported":
		代码 = 1005
	case "busy":
		代码 = 1006
	case "cancelled":
		代码 = 1007
	case "timeout":
		代码 = 1008
	case "output_limit":
		代码 = 1010
	case "closing":
		代码 = 1011
	case "image_read", "image_format", "image_limit", "input_limit", "attachment_unavailable":
		代码 = 1013
	case "credentials_unavailable":
		代码 = 1012
	}
	数据 := map[string]any{"kind": 当前失败.类别}
	if 当前失败.供应商请求标识 != "" {
		数据["provider_request_id"] = 当前失败.供应商请求标识
	}
	if 当前失败.HTTP状态 != 0 {
		数据["http_status"] = 当前失败.HTTP状态
	}
	return &协议包.错误{代码: 代码, 消息: 当前失败.消息, 数据: 数据}
}
```
