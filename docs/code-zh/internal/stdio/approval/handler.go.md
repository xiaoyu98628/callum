```go
package 审批包

import (
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	审批包 "github.com/xiaoyu98628/callum/internal/agent/approval"
	协议包 "github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

// 处理器 只转换协议参数，决定校验和事件发布由审批业务服务负责。
type 处理器 struct{ 服务 *审批包.服务 }

func 新建(服务 *审批包.服务) *处理器 { return &处理器{服务: 服务} }
func (当前处理器 *处理器) 句柄(执行上下文 上下文包.上下文, 请求 协议包.请求) (any, *协议包.错误) {
	var 值 any
	var 错误 error
	switch 请求.方法 {
	case "approval.get":
		var 当前对象 struct {
			标识 string `json:"id"`
		}
		if RPC错误 := 协议包.解码参数(请求.参数, &当前对象); RPC错误 != nil {
			return nil, RPC错误
		}
		值, 错误 = 当前处理器.服务.获取(执行上下文, 当前对象.标识)
	case "approval.list":
		var 当前对象 智能体包.审批筛选
		if RPC错误 := 协议包.解码参数(请求.参数, &当前对象); RPC错误 != nil {
			return nil, RPC错误
		}
		值, 错误 = 当前处理器.服务.列表(执行上下文, 当前对象)
	case "approval.decide":
		var 当前对象 struct {
			审批标识 string         `json:"approval_id"`
			决定   string         `json:"decision"`
			说明   *string        `json:"decision_note,omitempty"`
			范围   JSON包.原始JSON消息 `json:"scope,omitempty"`
		}
		if RPC错误 := 协议包.解码参数(请求.参数, &当前对象); RPC错误 != nil {
			return nil, RPC错误
		}
		范围 := 智能体包.审批一次
		if len(当前对象.范围) > 0 {
			var 已选择 *string
			if JSON包.反序列化(当前对象.范围, &已选择) != nil || 已选择 == nil || (*已选择 != 智能体包.审批一次 && *已选择 != 智能体包.审批会话目录) {
				return nil, 协议包.无效参数()
			}
			范围 = *已选择
		}
		值, 错误 = 当前处理器.服务.决定(执行上下文, 当前对象.审批标识, 当前对象.决定, 当前对象.说明, 范围)
	case "approval.directory.list":
		var 当前对象 struct {
			会话标识 string `json:"conversation_id"`
			上限   int    `json:"limit,omitempty"`
			偏移   int    `json:"offset,omitempty"`
		}
		if RPC错误 := 协议包.解码参数(请求.参数, &当前对象); RPC错误 != nil {
			return nil, RPC错误
		}
		值, 错误 = 当前处理器.服务.列表目录集(执行上下文, 当前对象.会话标识, 当前对象.上限, 当前对象.偏移)
	case "approval.directory.revoke":
		var 当前对象 struct {
			会话标识 string `json:"conversation_id"`
			标识   string `json:"id"`
		}
		if RPC错误 := 协议包.解码参数(请求.参数, &当前对象); RPC错误 != nil {
			return nil, RPC错误
		}
		值, 错误 = 当前处理器.服务.撤销目录(执行上下文, 当前对象.会话标识, 当前对象.标识)
	default:
		return nil, 协议包.方法未找到()
	}
	if 错误 == nil {
		return 值, nil
	}
	switch {
	case 错误判断包.是否属于(错误, 智能体包.审批无效错误):
		return nil, 协议包.无效参数()
	case 错误判断包.是否属于(错误, 智能体包.未找到错误):
		return nil, &协议包.错误{代码: 1001, 消息: "审批不存在"}
	case 错误判断包.是否属于(错误, 智能体包.冲突错误):
		return nil, &协议包.错误{代码: 1003, 消息: "审批状态已变化，无法提交该决定"}
	default:
		return nil, &协议包.错误{代码: -32603, 消息: "审批处理失败"}
	}
}
```
