```go
package 应用模块

import (
	JSON包 "encoding/json"
	输入输出包 "io"
	HTTP包 "net/http"
	HTTP测试包 "net/http/httptest"
	操作系统包 "os"
	文件路径包 "path/filepath"
	字符串包 "strings"
	原子操作包 "sync/atomic"
	测试包 "testing"
	时间包 "time"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	配置模块包 "github.com/xiaoyu98628/callum/internal/config"
	推理调用包 "github.com/xiaoyu98628/callum/internal/inference"
)

func 初始化工具工作区(测试器 *测试包.测试器, 当前应用通信对端 *应用通信对端) string {
	测试器.标记测试辅助函数()
	根 := 测试器.创建临时目录()
	if 错误 := 操作系统包.写入文件(文件路径包.合并(根, "a.txt"), []byte("text"), 0600); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	return 结果字符串(测试器, 当前应用通信对端.调用(测试器, "workspace.create", map[string]any{"name": "工作区", "root_path": 根}), "id")
}

func Test智能体继续之后已拒绝重复操作(测试器 *测试包.测试器) {
	测试器.并行()
	var 请求集 原子操作包.整数32
	上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
		if 请求集.添加(1) == 12 {
			写入文件工具响应(测试器, 当前响应写入器, false, map[string]any{"content": "已停止重复操作"}, "stop")
			return
		}
		当前响应写入器.头部().设置("Content-Type", "application/json")
		_, _ = 输入输出包.写入字符串(当前响应写入器, `{"choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"call","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"a.txt\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"total_tokens":1}}`)
	}))
	测试器.注册清理函数(上游.关闭)
	启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	启动配置.最大并发执行集 = 1
	当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	设置审批策略(测试器, 当前对象, "always")
	消息标识 := 初始化会话模型(测试器, 当前对象, 上游.URL)
	工作区标识 := 初始化工具工作区(测试器, 当前对象)
	已受理 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "workspace_id": 工作区标识, "stream": false, "content": 消息内容块集("读取")})
	执行标识 := 结果字符串(测试器, 已受理, "run_id")
	事件 := 当前对象.事件(测试器, "approval.requested")
	var 载荷 struct {
		标识 string `json:"approval_id"`
	}
	if 错误 := JSON包.反序列化(事件.参数, &载荷); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	要求成功(测试器, 当前对象.调用(测试器, "approval.decide", map[string]any{"approval_id": 载荷.标识, "decision": "rejected"}))
	已完成 := 当前对象.事件(测试器, "run.completed")
	var 结果 struct {
		执行 智能体包.执行 `json:"run"`
	}
	if 错误 := JSON包.反序列化(已完成.参数, &结果); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 请求集.加载() != 12 || 结果.执行.状态 != 智能体包.已完成 || 结果.执行.错误类别 != nil {
		测试器.格式化报告致命错误("超过原轮次限制后未完成: %d %+v", 请求集.加载(), 结果.执行)
	}
	记录集 := 当前对象.调用(测试器, "approval.list", map[string]any{"run_id": 执行标识})
	要求成功(测试器, 记录集)
	var 行集 []智能体包.审批
	if 错误 := JSON包.反序列化(记录集.结果, &行集); 错误 != nil || len(行集) != 1 {
		测试器.格式化报告致命错误("相同拒绝操作重新申请审批: %s %v", 记录集.结果, 错误)
	}
	当前对象.停止(测试器)
}

func Test连续智能体能够取消或者关闭超出前一个上限(测试器 *测试包.测试器) {
	测试器.并行()
	for _, 动作 := range []string{"cancel", "shutdown"} {
		测试器.执行(动作, func(测试器 *测试包.测试器) {
			var 请求集 原子操作包.整数32
			已开始 := make(chan struct{}, 1)
			上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
				// 读完请求体后 HTTP 服务才能可靠感知客户端断连；测试结束也必须释放等待。
				if _, 错误 := 输入输出包.复制(输入输出包.丢弃, 当前请求.正文); 错误 != nil {
					测试器.错误文本(错误)
					return
				}
				if 请求集.添加(1) == 10 {
					已开始 <- struct{}{}
					select {
					case <-当前请求.上下文().完成():
					case <-测试器.上下文().完成():
					}
					return
				}
				写入文件工具响应(测试器, 当前响应写入器, false, map[string]any{"tool_calls": []any{map[string]any{"id": "call", "type": "function", "function": map[string]string{"name": "missing_tool", "arguments": `{}`}}}}, "tool_calls")
			}))
			测试器.注册清理函数(上游.关闭)
			启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			启动配置.最大并发执行集 = 1
			当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
			消息标识 := 初始化会话模型(测试器, 当前对象, 上游.URL)
			响应 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "stream": false, "content": 消息内容块集("持续调用")})
			执行标识 := 结果字符串(测试器, 响应, "run_id")
			// 持续消费事件，不能让未读取的 标准输入输出 输出阻塞服务而使取消测试失真。
			for {
				帧 := 当前对象.读取(测试器)
				当前对象.待处理 = append(当前对象.待处理, 帧)
				if 帧.方法 == "run.failed" || 帧.方法 == "run.completed" {
					测试器.格式化报告致命错误("持续执行提前结束: %s", 帧.参数)
				}
				if 帧.方法 == "step.updated" && 字符串包.包含(string(帧.参数), `"round":10`) {
					break
				}
			}
			计时器 := 时间包.新建计时器(3 * 时间包.秒)
			defer 计时器.停止()
			select {
			case <-已开始:
			case <-计时器.字符:
				测试器.报告致命错误("未进入第十轮模型请求")
			}
			if 动作 == "cancel" {
				要求成功(测试器, 当前对象.调用(测试器, "run.cancel", map[string]any{"run_id": 执行标识}))
				当前对象.事件(测试器, "run.cancelled")
				帧 := 当前对象.调用(测试器, "step.list", map[string]any{"run_id": 执行标识})
				要求成功(测试器, 帧)
				var 步骤集 []智能体包.步骤摘要
				if 错误 := JSON包.反序列化(帧.结果, &步骤集); 错误 != nil {
					测试器.报告致命错误(错误)
				}
				工具集 := 0
				for _, 步骤 := range 步骤集 {
					if 步骤.类型 == "tool_call" {
						工具集++
						if 步骤.输出字节 == 0 {
							测试器.报告致命错误("取消清除了已发生工具结果")
						}
					}
				}
				if 工具集 != 9 {
					测试器.格式化报告致命错误("工具审计不完整: %d", 工具集)
				}
				当前对象.停止(测试器)
			} else {
				当前对象.停止(测试器)
				已取消 := false
				for _, 帧 := range 当前对象.待处理 {
					已取消 = 已取消 || 帧.方法 == "run.cancelled"
				}
				if !已取消 {
					测试器.报告致命错误("关闭应用未取消持续执行")
				}
			}
			if 请求集.加载() != 10 {
				测试器.格式化报告致命错误("取消或关闭后仍发起请求: %d", 请求集.加载())
			}
		})
	}
}

func Test已暂停智能体隔离并且关闭(测试器 *测试包.测试器) {
	上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
		var 输入 struct {
			工具集 []JSON包.原始JSON消息          `json:"tools"`
			消息集 []struct{ 角色, 内容 string } `json:"messages"`
		}
		if 错误 := JSON包.新建解码器(当前请求.正文).解码(&输入); 错误 != nil {
			测试器.错误文本(错误)
			return
		}
		当前响应写入器.头部().设置("Content-Type", "application/json")
		if len(输入.工具集) == 0 || 输入.消息集[len(输入.消息集)-1].内容 == "直接回答" {
			_, _ = 输入输出包.写入字符串(当前响应写入器, `{"choices":[{"index":0,"message":{"role":"assistant","content":"回答"},"finish_reason":"stop"}]}`)
			return
		}
		_, _ = 输入输出包.写入字符串(当前响应写入器, `{"choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"call","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"a.txt\"}"}}]},"finish_reason":"tool_calls"}]}`)
	}))
	测试器.注册清理函数(上游.关闭)
	启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	启动配置.最大并发执行集 = 2
	当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	设置审批策略(测试器, 当前对象, "always")
	消息标识 := 初始化会话模型(测试器, 当前对象, 上游.URL)
	工作区标识 := 初始化工具工作区(测试器, 当前对象)
	首个 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "workspace_id": 工作区标识, "stream": false, "content": 消息内容块集("读取")})
	首个标识 := 结果字符串(测试器, 首个, "run_id")
	会话标识 := 结果字符串(测试器, 首个, "conversation_id")
	事件 := 当前对象.事件(测试器, "approval.requested")
	var 载荷 struct {
		标识 string `json:"approval_id"`
	}
	if 错误 := JSON包.反序列化(事件.参数, &载荷); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	繁忙 := 当前对象.调用(测试器, "conversation.send", map[string]any{"conversation_id": 会话标识, "content": 消息内容块集("重复")})
	if 繁忙.错误 == nil || 繁忙.错误.代码 != 1006 {
		测试器.报告致命错误("等待审批时允许同会话并发")
	}
	秒 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "workspace_id": 工作区标识, "stream": false, "content": 消息内容块集("直接回答")})
	秒标识 := 结果字符串(测试器, 秒, "run_id")
	已完成 := 当前对象.事件(测试器, "run.completed")
	var 终态 struct {
		标识 string `json:"run_id"`
	}
	if 错误 := JSON包.反序列化(已完成.参数, &终态); 错误 != nil || 终态.标识 != 秒标识 {
		测试器.报告致命错误("审批阻塞了其他会话")
	}
	状态 := 当前对象.调用(测试器, "run.get", map[string]any{"id": 首个标识})
	要求成功(测试器, 状态)
	var 已暂停 智能体包.执行
	if 错误 := JSON包.反序列化(状态.结果, &已暂停); 错误 != nil || 已暂停.状态 != 智能体包.等待中审批 {
		测试器.报告致命错误("等待审批状态丢失")
	}
	当前对象.停止(测试器)
	已恢复 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	记录 := 已恢复.调用(测试器, "approval.get", map[string]any{"id": 载荷.标识})
	要求成功(测试器, 记录)
	var 甲 智能体包.审批
	if 错误 := JSON包.反序列化(记录.结果, &甲); 错误 != nil || 甲.状态 != 智能体包.审批已取消 {
		测试器.格式化报告致命错误("关闭后待办未取消: %s", 记录.结果)
	}
	已恢复.停止(测试器)
}

func Test不完整工具流不执行(测试器 *测试包.测试器) {
	上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
		当前响应写入器.头部().设置("Content-Type", "text/event-stream")
		_, _ = 输入输出包.写入字符串(当前响应写入器, "data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call\",\"type\":\"function\",\"function\":{\"name\":\"read_file\",\"arguments\":\"{\\\"path\\\":\"}}]}}]}\n\ndata: {\"choices\":[],\"usage\":{\"total_tokens\":2}}\n\ndata: [DONE]\n\n")
	}))
	测试器.注册清理函数(上游.关闭)
	启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	启动配置.最大并发执行集 = 1
	当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	设置审批策略(测试器, 当前对象, "always")
	消息标识 := 初始化会话模型(测试器, 当前对象, 上游.URL)
	工作区标识 := 初始化工具工作区(测试器, 当前对象)
	已受理 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "workspace_id": 工作区标识, "stream": true, "content": 消息内容块集("读取")})
	执行标识 := 结果字符串(测试器, 已受理, "run_id")
	当前对象.事件(测试器, "run.failed")
	记录集 := 当前对象.调用(测试器, "approval.list", map[string]any{"run_id": 执行标识})
	要求成功(测试器, 记录集)
	var 审批集 []智能体包.审批
	if 错误 := JSON包.反序列化(记录集.结果, &审批集); 错误 != nil || len(审批集) != 0 {
		测试器.报告致命错误("不完整参数触发审批")
	}
	步骤集帧 := 当前对象.调用(测试器, "step.list", map[string]any{"run_id": 执行标识})
	要求成功(测试器, 步骤集帧)
	var 步骤集 []智能体包.步骤摘要
	if 错误 := JSON包.反序列化(步骤集帧.结果, &步骤集); 错误 != nil || len(步骤集) != 1 || 步骤集[0].类型 != "model_call" || 步骤集[0].状态 != 智能体包.失败 || 步骤集[0].用量字节 == 0 {
		测试器.格式化报告致命错误("不完整参数触发工具步骤或丢失 usage: %s", 步骤集帧.结果)
	}
	当前对象.停止(测试器)
}

func Test智能体流累计文本上限(测试器 *测试包.测试器) {
	for _, 超过上限 := range []bool{false, true} {
		测试器.执行(map[bool]string{false: "exact_limit", true: "over_limit"}[超过上限], func(测试器 *测试包.测试器) {
			初始, 前缀 := 字符串包.重复("a", 600_000), "保留"
			剩余 := 推理调用包.最大文本字节 - len(初始) - len(前缀)
			if 超过上限 {
				剩余++
			}
			最后 := 字符串包.重复("b", 剩余)
			var 请求集 原子操作包.整数32
			上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
				当前响应写入器.头部().设置("X-Request-ID", "limit-request")
				switch 请求集.添加(1) {
				case 1:
					写入文件工具响应(测试器, 当前响应写入器, true, map[string]any{"content": 初始, "tool_calls": []any{map[string]any{"index": 0, "id": "read", "type": "function", "function": map[string]string{"name": "read_file", "arguments": `{"path":"a.txt"}`}}}}, "tool_calls")
				case 2:
					当前响应写入器.头部().设置("Content-Type", "text/event-stream")
					if _, 错误 := 输入输出包.写入字符串(当前响应写入器, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"保留\"},\"finish_reason\":null}]}\n\n"); 错误 != nil {
						测试器.错误文本(错误)
						return
					}
					写入文件工具响应(测试器, 当前响应写入器, true, map[string]any{"content": 最后}, "stop")
				default:
					测试器.错误文本("文本超限后仍继续调用模型")
					当前响应写入器.写入头部(HTTP包.状态内部服务端错误)
				}
			}))
			测试器.注册清理函数(上游.关闭)
			启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			启动配置.最大并发执行集 = 1
			当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
			消息标识 := 初始化会话模型(测试器, 当前对象, 上游.URL)
			工作区标识 := 初始化工具工作区(测试器, 当前对象)
			已受理 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "workspace_id": 工作区标识, "stream": true, "content": 消息内容块集("读取并回答")})
			执行标识, 会话标识 := 结果字符串(测试器, 已受理, "run_id"), 结果字符串(测试器, 已受理, "conversation_id")
			终态, 预期文本, 预期字节 := "run.completed", 前缀+最后, 推理调用包.最大文本字节
			if 超过上限 {
				终态, 预期文本, 预期字节 = "run.failed", 前缀, len(初始)+len(前缀)
			}
			帧 := 当前对象.事件(测试器, 终态)
			var 最终 struct {
				执行 智能体包.执行 `json:"run"`
			}
			if 错误 := JSON包.反序列化(帧.参数, &最终); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			if 最终.执行.标识 != 执行标识 || (超过上限 && (最终.执行.错误类别 == nil || *最终.执行.错误类别 != "output_limit")) {
				测试器.格式化报告致命错误("执行终态错误: %s", 帧.参数)
			}
			消息集 := 读取消息集(测试器, 当前对象, 会话标识)
			if len(消息集) != 2 || len(消息集[1].内容) != 1 || 消息集[1].内容[0].文本 != 预期文本 || 消息集[1].状态 != 最终.执行.状态 {
				测试器.报告致命错误("保存的正文与通过累计限额的增量不一致")
			}
			增量字节 := 0
			for _, 事件 := range 当前对象.待处理 {
				if 事件.方法 == "message.delta" {
					var 值 struct{ 增量 struct{ 文本 string } }
					if 错误 := JSON包.反序列化(事件.参数, &值); 错误 != nil {
						测试器.报告致命错误(错误)
					}
					增量字节 += len(值.增量.文本)
				}
				if 事件.方法 == "message.updated" && 超过上限 {
					var 值 struct {
						消息 struct{ 内容 []struct{ 文本 string } }
					}
					if 错误 := JSON包.反序列化(事件.参数, &值); 错误 != nil {
						测试器.报告致命错误(错误)
					}
					for _, 内容块 := range 值.消息.内容 {
						if 字符串包.包含(内容块.文本, "b") {
							测试器.报告致命错误("被拒收的增量通过消息更新返回")
						}
					}
				}
			}
			if 增量字节 != 预期字节 || 请求集.加载() != 2 {
				测试器.格式化报告致命错误("累计增量或调用轮数错误: bytes=%d requests=%d", 增量字节, 请求集.加载())
			}
			步骤集帧 := 当前对象.调用(测试器, "step.list", map[string]any{"run_id": 执行标识})
			要求成功(测试器, 步骤集帧)
			var 步骤集 []智能体包.步骤摘要
			if 错误 := JSON包.反序列化(步骤集帧.结果, &步骤集); 错误 != nil || len(步骤集) != 3 {
				测试器.格式化报告致命错误("模型和工具步骤不完整: %v", 错误)
			}
			步骤 := 步骤集[2]
			if 步骤.状态 != 最终.执行.状态 || 步骤.用量字节 == 0 || 步骤.供应商请求标识 == nil || *步骤.供应商请求标识 != "limit-request" {
				测试器.格式化报告致命错误("模型步骤终态或元数据丢失: %+v", 步骤)
			}
			for _, 字段 := range []string{"output", "usage"} {
				数据 := 当前对象.调用(测试器, "step.data", map[string]any{"step_id": 步骤.标识, "field": 字段})
				要求成功(测试器, 数据)
				var 值 struct{ 片段 string }
				if 错误 := JSON包.反序列化(数据.结果, &值); 错误 != nil {
					测试器.报告致命错误(错误)
				}
				if (字段 == "output" && !字符串包.包含(值.片段, 字符串包.重复("b", 64))) || (字段 == "usage" && !字符串包.包含(值.片段, `"total_tokens":1`)) {
					测试器.格式化报告致命错误("原始响应或实际 usage 丢失: %s", 字段)
				}
			}
			当前对象.停止(测试器)
		})
	}
}

func Test智能体拒绝音频响应(测试器 *测试包.测试器) {
	for _, 流 := range []bool{false, true} {
		测试器.执行(map[bool]string{false: "generate", true: "stream"}[流], func(测试器 *测试包.测试器) {
			上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
				写入文件工具响应(测试器, 当前响应写入器, 流, map[string]any{"content": "", "audio": map[string]string{"id": "audio-1", "data": "ZmFrZQ=="}}, "stop")
			}))
			测试器.注册清理函数(上游.关闭)
			启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			启动配置.最大并发执行集 = 1
			当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
			消息标识 := 初始化会话模型(测试器, 当前对象, 上游.URL)
			已受理 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "stream": 流, "content": 消息内容块集("回答")})
			帧 := 当前对象.事件(测试器, "run.failed")
			var 最终 struct{ 执行 智能体包.执行 }
			if 错误 := JSON包.反序列化(帧.参数, &最终); 错误 != nil || 最终.执行.错误类别 == nil || *最终.执行.错误类别 != "unsupported" {
				测试器.格式化报告致命错误("音频响应没有明确失败: %s %v", 帧.参数, 错误)
			}
			消息集 := 读取消息集(测试器, 当前对象, 结果字符串(测试器, 已受理, "conversation_id"))
			if len(消息集) != 2 || 消息集[1].状态 != 智能体包.失败 {
				测试器.报告致命错误("不支持的响应被保存为成功回答")
			}
			当前对象.停止(测试器)
			for _, 事件 := range 当前对象.待处理 {
				if 事件.方法 == "run.completed" {
					测试器.报告致命错误("音频响应仍发出成功终态")
				}
			}
		})
	}
}

// 设置审批策略 通过公开通信接口修改持久化配置，验证不会依赖启动时注入。
func 设置审批策略(测试器 *测试包.测试器, 当前应用通信对端 *应用通信对端, 选项 string) {
	测试器.标记测试辅助函数()
	当前 := 当前应用通信对端.调用(测试器, "settings.get", map[string]any{"key": "execution"})
	要求成功(测试器, 当前)
	var 值 struct {
		修订号 int64 `json:"revision"`
	}
	if 错误 := JSON包.反序列化(当前.结果, &值); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	要求成功(测试器, 当前应用通信对端.调用(测试器, "settings.update", map[string]any{"key": "execution", "revision": 值.修订号,
		"value": map[string]any{"approval_policy": 选项, "approvals_reviewer": "user"}}))
}
```
