```go
package 执行

import (
	映射包 "maps"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	会话包 "github.com/xiaoyu98628/callum/internal/conversation"
	推理调用包 "github.com/xiaoyu98628/callum/internal/inference"
	协议包 "github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

// 步骤摘要 保持步骤查询与通知使用相同的元数据结构。
type 步骤摘要 = 智能体包.步骤摘要

func 摘要(步骤 智能体包.步骤) 步骤摘要 { return 智能体包.生成摘要步骤(步骤) }

// 发布持久化 在 标准输入输出 边界将业务事件编码为兼容的 JSON-RPC 通知。
func 发布持久化(通知 func(string, any) error, 事件 智能体包.事件) error {
	if 数据, 成功 := 事件.数据.(推理调用包.通知); 成功 {
		参数 := struct {
			执行标识    string  `json:"run_id"`
			文本      *string `json:"text,omitempty"`
			供应商请求标识 string  `json:"provider_request_id,omitempty"`
			错误      *协议包.错误 `json:"error,omitempty"`
		}{执行标识: 事件.执行标识, 文本: 数据.文本, 供应商请求标识: 数据.供应商请求标识}
		if 数据.失败 != nil {
			参数.错误 = 协议错误(*数据.失败)
		}
		return 通知(事件.类型, 参数)
	}
	参数 := map[string]any{}
	if 事件.状态已持久化 != nil {
		参数["persisted"] = *事件.状态已持久化
	}
	if len(事件.已移除消息标识集) > 0 {
		参数["removed_message_ids"] = 事件.已移除消息标识集
	}
	if 事件.会话标识 != "" {
		参数["conversation_id"] = 事件.会话标识
	}
	if 事件.执行标识 != "" {
		参数["run_id"] = 事件.执行标识
	}
	if 事件.步骤标识 != "" {
		参数["step_id"] = 事件.步骤标识
	}
	if 事件.消息标识 != "" {
		参数["message_id"] = 事件.消息标识
	}
	switch 值 := 事件.数据.(type) {
	case 会话包.上下文用量:
		参数["context_usage"] = 值
	case 会话包.会话:
		参数["conversation"] = 值
	case 会话包.消息:
		参数["message"] = 值
	case 智能体包.步骤:
		参数["step"] = 摘要(值)
	case 智能体包.步骤摘要:
		参数["step"] = 值
	case 智能体包.工具输出:
		参数["output"] = 值
	case 智能体包.用户输入:
		参数["input_id"] = 值.标识
		参数["user_input"] = 值
	case 智能体包.审批:
		参数["approval_id"] = 值.标识
		参数["approval"] = 值
	case 智能体包.目录授权:
		参数["directory_grant"] = 值
	case 智能体包.执行:
		参数["run"] = 值
		if 值.错误类别 != nil && 值.错误消息 != nil {
			参数["error"] = map[string]any{"kind": *值.错误类别, "message": *值.错误消息}
		}
	case map[string]any:
		映射包.复制(参数, 值)
	}
	return 通知(事件.类型, 参数)
}
```
