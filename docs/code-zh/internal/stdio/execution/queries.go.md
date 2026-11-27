```go
package 执行

import (
	上下文包 "context"
	错误判断包 "errors"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	协议包 "github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

func 查询错误(错误 error) *协议包.错误 {
	已转换 := 错误(错误)
	if 已转换 != nil && 已转换.代码 == 1009 {
		return &协议包.错误{代码: -32603, 消息: "执行数据查询失败"}
	}
	return 已转换
}

// 查询 仅处理持久化执行查询；标识不能使用旧无历史调用的临时 执行标识。
func (当前处理器 *处理器) 查询(执行上下文 上下文包.上下文, 请求 协议包.请求) (any, *协议包.错误) {
	if 当前处理器.持久化 == nil {
		return nil, 协议包.方法未找到()
	}
	switch 请求.方法 {
	case "run.get", "step.get":
		var 当前对象 struct {
			标识 string `json:"id"`
		}
		if 错误 := 协议包.解码参数(请求.参数, &当前对象); 错误 != nil {
			return nil, 错误
		}
		if 请求.方法 == "run.get" {
			值, 错误 := 当前处理器.持久化.获取(执行上下文, 当前对象.标识)
			return 值, 查询错误(错误)
		}
		值, 错误 := 当前处理器.持久化.获取步骤摘要(执行上下文, 当前对象.标识)
		return 值, 查询错误(错误)
	case "run.list":
		var 当前对象 struct {
			会话标识 string `json:"conversation_id"`
			上限   int    `json:"limit,omitempty"`
			偏移   int    `json:"offset,omitempty"`
		}
		if 错误 := 协议包.解码参数(请求.参数, &当前对象); 错误 != nil {
			return nil, 错误
		}
		if 当前对象.上限 == 0 {
			当前对象.上限 = 100
		}
		值, 错误 := 当前处理器.持久化.列表(执行上下文, 当前对象.会话标识, 当前对象.上限, 当前对象.偏移)
		return 值, 查询错误(错误)
	case "step.list":
		var 当前对象 struct {
			执行标识 string `json:"run_id"`
			之后序号 int64  `json:"after_sequence,omitempty"`
			上限   int    `json:"limit,omitempty"`
		}
		if 错误 := 协议包.解码参数(请求.参数, &当前对象); 错误 != nil {
			return nil, 错误
		}
		if 当前对象.上限 == 0 {
			当前对象.上限 = 100
		}
		行集, 错误 := 当前处理器.持久化.步骤摘要集(执行上下文, 当前对象.执行标识, 当前对象.之后序号, 当前对象.上限)
		return 行集, 查询错误(错误)
	case "step.data":
		var 当前对象 struct {
			步骤标识 string `json:"step_id"`
			字段   string `json:"field"`
			偏移   int    `json:"offset,omitempty"`
			上限   int    `json:"limit,omitempty"`
		}
		if 错误 := 协议包.解码参数(请求.参数, &当前对象); 错误 != nil {
			return nil, 错误
		}
		if 当前对象.偏移 < 0 {
			return nil, 协议包.无效参数()
		}
		if 当前对象.上限 == 0 {
			当前对象.上限 = 64 << 10
		}
		if 当前对象.上限 < 4 || 当前对象.上限 > 64<<10 {
			return nil, 协议包.无效参数()
		}
		数据, 错误 := 当前处理器.持久化.步骤数据(执行上下文, 当前对象.步骤标识, 当前对象.字段, 当前对象.偏移, 当前对象.上限)
		if 错误判断包.是否属于(错误, 智能体包.步骤推理错误) {
			return nil, &协议包.错误{代码: -32603, 消息: 智能体包.步骤推理错误.错误文本()}
		}
		if 错误 != nil {
			return nil, 查询错误(错误)
		}
		return map[string]any{"step_id": 当前对象.步骤标识, "field": 当前对象.字段, "present": 数据.存在, "offset": 当前对象.偏移,
			"next_offset": 数据.下一个偏移, "total_bytes": 数据.总计字节, "done": 数据.下一个偏移 == 数据.总计字节, "fragment": 数据.片段}, nil
	default:
		return nil, 协议包.方法未找到()
	}
}
```
