```go
package 执行

import (
	上下文包 "context"
	时间包 "time"

	智能体服务包 "github.com/xiaoyu98628/callum/internal/agent/service"
	推理调用包 "github.com/xiaoyu98628/callum/internal/inference"
	执行器包 "github.com/xiaoyu98628/callum/internal/inference/executor"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	供应商包 "github.com/xiaoyu98628/callum/internal/model/provider"
	协议包 "github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

// 处理器 处理模型调用和独立取消，不负责创建或关闭执行器。
type 处理器 struct {
	执行集       *执行器包.管理器
	持久化 *智能体服务包.引擎
	已受理   func(string) error
}

// 新建 使用已初始化的非 nil 执行器创建接口处理器，执行器由调用方持有。
func 新建(执行集 *执行器包.管理器) *处理器 { return &处理器{执行集: 执行集} }

// 新建持久化 使用共享调度器入口和持久化运行服务。
func 新建持久化(执行集 *执行器包.管理器, 持久化 *智能体服务包.引擎, 已受理 func(string) error) *处理器 {
	return &处理器{执行集: 执行集, 持久化: 持久化, 已受理: 已受理}
}

// 上限集 返回执行器的并发上限与单次调用超时。
func (当前处理器 *处理器) 上限集() (int, 时间包.时长) { return 当前处理器.执行集.上限集() }

// 调用 接受调用并异步执行；回复 必须在返回前将响应排在事件之前。
func (当前处理器 *处理器) 调用(执行上下文 上下文包.上下文, 请求 协议包.请求, 回复 func(any, *协议包.错误) error) error {
	var 当前对象 调用参数
	if 错误 := 协议包.解码参数(请求.参数, &当前对象); 错误 != nil {
		return 回复(nil, 错误)
	}
	if !供应商包.有效标识(当前对象.模型标识) {
		return 回复(nil, 协议包.无效参数())
	}
	输入 := 推理调用包.请求{模型标识: 当前对象.模型标识, 流: true}
	if 当前对象.流 != nil {
		输入.流 = *当前对象.流
	}
	for _, 消息 := range 当前对象.消息集 {
		if 消息.内容 == nil {
			return 回复(nil, 协议包.无效参数())
		}
		输入.消息集 = append(输入.消息集, 模型包.消息{角色: 消息.角色, 内容: *消息.内容})
	}
	错误 := 当前处理器.执行集.启动(执行上下文, 输入, func(标识 string) error {
		if 当前处理器.已受理 != nil {
			if 错误 := 当前处理器.已受理(标识); 错误 != nil {
				return 错误
			}
		}
		return 回复(map[string]any{"run_id": 标识}, nil)
	})
	if 错误 != nil {
		return 回复(nil, 公开错误(错误))
	}
	return nil
}

// 取消执行 校验取消参数，返回请求取消的结果，不等待模型执行结束。
func (当前处理器 *处理器) 取消执行(请求 协议包.请求) (any, *协议包.错误) {
	var 当前对象 取消参数
	if 错误 := 协议包.解码参数(请求.参数, &当前对象); 错误 != nil {
		return nil, 错误
	}
	if 当前对象.执行标识 == "" {
		return nil, 协议包.无效参数()
	}
	return map[string]any{"cancel_requested": 当前处理器.执行集.取消执行(当前对象.执行标识)}, nil
}
```
