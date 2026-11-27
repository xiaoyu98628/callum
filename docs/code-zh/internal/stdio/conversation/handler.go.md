```go
package 会话

import (
	上下文包 "context"

	业务域包 "github.com/xiaoyu98628/callum/internal/conversation"
	服务包 "github.com/xiaoyu98628/callum/internal/conversation/service"
	协议包 "github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

// 处理器 转换对话接口；已受理 登记本连接发起的执行。
type 处理器 struct {
	服务  *服务包.服务
	已受理 func(string) error
}

func 新建(当前服务 *服务包.服务, 已受理 func(string) error) *处理器 {
	return &处理器{服务包: 当前服务, 已受理: 已受理}
}

func (当前处理器 *处理器) 句柄(执行上下文 上下文包.上下文, 请求 协议包.请求, 回复 func(any, *协议包.错误) error) error {
	if 请求.方法 == "conversation.compact" {
		return 当前处理器.压缩(执行上下文, 请求, 回复)
	}
	if 请求.方法 == "conversation.send" || 请求.方法 == "conversation.resend" || 请求.方法 == "conversation.follow_up_start" || 请求.方法 == "conversation.continue" {
		return 当前处理器.内部发送(执行上下文, 请求, 回复)
	}
	var 值 any
	var 错误 error
	switch 请求.方法 {
	case "conversation.follow_ups":
		var 当前对象 struct {
			会话标识 string `json:"conversation_id"`
		}
		if RPC错误 := 协议包.解码参数(请求.参数, &当前对象); RPC错误 != nil {
			return 回复(nil, RPC错误)
		}
		值, 错误 = 当前处理器.服务.后续消息集(执行上下文, 当前对象.会话标识)
	case "conversation.follow_up_add":
		var 当前对象 业务域包.后续消息输入
		if RPC错误 := 协议包.解码参数(请求.参数, &当前对象); RPC错误 != nil {
			return 回复(nil, RPC错误)
		}
		值, 错误 = 当前处理器.服务.添加后续消息(执行上下文, 当前对象)
	case "conversation.follow_up_change":
		var 当前对象 业务域包.后续消息变更
		if RPC错误 := 协议包.解码参数(请求.参数, &当前对象); RPC错误 != nil {
			return 回复(nil, RPC错误)
		}
		值, 错误 = 当前处理器.服务.变更后续消息(执行上下文, 当前对象)
	case "conversation.attachment_read":
		var 当前对象 struct {
			会话标识   string `json:"conversation_id"`
			消息标识   string `json:"message_id"`
			预期执行标识 string `json:"expected_run_id,omitempty"`
			索引     int    `json:"index"`
			偏移     int64  `json:"offset"`
		}
		if RPC错误 := 协议包.解码参数(请求.参数, &当前对象); RPC错误 != nil {
			return 回复(nil, RPC错误)
		}
		值, 错误 = 当前处理器.服务.读取附件分片(执行上下文, 当前对象.会话标识, 当前对象.消息标识, 当前对象.预期执行标识, 当前对象.索引, 当前对象.偏移)
	case "conversation.history":
		var 当前对象 struct {
			会话标识   string `json:"conversation_id"`
			之前执行标识 string `json:"before_run_id,omitempty"`
			上限     int    `json:"limit,omitempty"`
		}
		if RPC错误 := 协议包.解码参数(请求.参数, &当前对象); RPC错误 != nil {
			return 回复(nil, RPC错误)
		}
		值, 错误 = 当前处理器.服务.历史(执行上下文, 当前对象.会话标识, 当前对象.之前执行标识, 当前对象.上限)
	case "conversation.attachment_preview":
		var 当前对象 struct {
			会话标识 string `json:"conversation_id,omitempty"`
			消息标识 string `json:"message_id,omitempty"`
			索引   int    `json:"index,omitempty"`
			路径   string `json:"path,omitempty"`
		}
		if RPC错误 := 协议包.解码参数(请求.参数, &当前对象); RPC错误 != nil {
			return 回复(nil, RPC错误)
		}
		数据, MIME, 预览错误 := 当前处理器.服务.预览附件(执行上下文, 当前对象.会话标识, 当前对象.消息标识, 当前对象.索引, 当前对象.路径)
		if 预览错误 != nil {
			return 回复(nil, 公开错误(预览错误))
		}
		return 回复(map[string]any{"data": 数据, "mime": MIME}, nil)
	case "conversation.context_usage":
		var 当前对象 业务域包.上下文用量请求
		if RPC错误 := 协议包.解码参数(请求.参数, &当前对象); RPC错误 != nil {
			return 回复(nil, RPC错误)
		}
		值, 错误 = 当前处理器.服务.上下文用量(执行上下文, 当前对象)
	case "conversation.context_versions":
		var 当前对象 struct {
			会话标识 string `json:"conversation_id"`
			上限   int    `json:"limit,omitempty"`
			偏移   int    `json:"offset,omitempty"`
		}
		if RPC错误 := 协议包.解码参数(请求.参数, &当前对象); RPC错误 != nil {
			return 回复(nil, RPC错误)
		}
		值, 错误 = 当前处理器.服务.上下文版本集(执行上下文, 当前对象.会话标识, 当前对象.上限, 当前对象.偏移)
	case "conversation.context_version":
		var 当前对象 struct {
			会话标识 string `json:"conversation_id"`
			标识   string `json:"id"`
		}
		if RPC错误 := 协议包.解码参数(请求.参数, &当前对象); RPC错误 != nil {
			return 回复(nil, RPC错误)
		}
		值, 错误 = 当前处理器.服务.上下文版本(执行上下文, 当前对象.会话标识, 当前对象.标识)
	case "conversation.list":
		var 当前对象 业务域包.筛选
		if RPC错误 := 协议包.解码参数(请求.参数, &当前对象); RPC错误 != nil {
			return 回复(nil, RPC错误)
		}
		值, 错误 = 当前处理器.服务.列表(执行上下文, 当前对象)
	case "conversation.get", "conversation.delete", "conversation.restore":
		var 当前对象 struct {
			标识 string `json:"id"`
		}
		if RPC错误 := 协议包.解码参数(请求.参数, &当前对象); RPC错误 != nil {
			return 回复(nil, RPC错误)
		}
		if 请求.方法 == "conversation.get" {
			值, 错误 = 当前处理器.服务.获取(执行上下文, 当前对象.标识)
		} else {
			值, 错误 = 当前处理器.服务.更新(执行上下文, 当前对象.标识, 业务域包.更新{已删除: new(请求.方法 == "conversation.delete")})
		}
	case "conversation.update":
		var 当前对象 struct {
			标识 string `json:"id"`
			业务域包.更新
		}
		if RPC错误 := 协议包.解码参数(请求.参数, &当前对象); RPC错误 != nil {
			return 回复(nil, RPC错误)
		}
		值, 错误 = 当前处理器.服务.更新(执行上下文, 当前对象.标识, 当前对象.更新)
	default:
		return 回复(nil, 协议包.方法未找到())
	}
	return 回复(值, 公开错误(错误))
}

func (当前处理器 *处理器) 内部发送(执行上下文 上下文包.上下文, 请求 协议包.请求, 回复 func(any, *协议包.错误) error) error {
	已回复 := false
	已受理 := func(本次执行 业务域包.执行) error {
		if 当前处理器.已受理 != nil {
			if 错误 := 当前处理器.已受理(本次执行.执行.标识); 错误 != nil {
				return 错误
			}
		}
		已回复 = true
		return 回复(map[string]any{"conversation_id": 本次执行.会话.标识, "run_id": 本次执行.执行.标识, "user_message_id": 本次执行.用户.标识, "message_id": 本次执行.助手.标识, "removed_message_ids": 本次执行.已移除消息标识集}, nil)
	}
	var 错误 error
	switch 请求.方法 {
	case "conversation.follow_up_start":
		var 当前对象 业务域包.后续消息开始
		if RPC错误 := 协议包.解码参数(请求.参数, &当前对象); RPC错误 != nil {
			return 回复(nil, RPC错误)
		}
		错误 = 当前处理器.服务.开始后续消息(执行上下文, 当前对象, 已受理)
	case "conversation.continue":
		var 当前对象 struct {
			业务域包.继续
			// 旧服务拒绝此参数，桌面端可避免向旧版发送会产生新消息的继续请求。
			继续原地 bool `json:"resume_in_place,omitempty"`
		}
		if RPC错误 := 协议包.解码参数(请求.参数, &当前对象); RPC错误 != nil {
			return 回复(nil, RPC错误)
		}
		错误 = 当前处理器.服务.继续(执行上下文, 当前对象.继续, 已受理)
	default:
		var 当前对象 业务域包.发送
		if RPC错误 := 协议包.解码参数(请求.参数, &当前对象); RPC错误 != nil {
			return 回复(nil, RPC错误)
		}
		if (请求.方法 == "conversation.resend") != (当前对象.消息标识 != "") {
			return 回复(nil, 协议包.无效参数())
		}
		错误 = 当前处理器.服务.发送(执行上下文, 当前对象, 已受理)
	}
	if 错误 != nil {
		if 已回复 {
			return 错误
		}
		return 回复(nil, 公开错误(错误))
	}
	return nil
}
```
