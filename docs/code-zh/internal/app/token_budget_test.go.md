```go
package 应用模块

import (
	JSON包 "encoding/json"
	格式化包 "fmt"
	输入输出包 "io"
	HTTP包 "net/http"
	HTTP测试包 "net/http/httptest"
	文件路径包 "path/filepath"
	字符串包 "strings"
	原子操作包 "sync/atomic"
	测试包 "testing"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	配置模块包 "github.com/xiaoyu98628/callum/internal/config"
	设置包 "github.com/xiaoyu98628/callum/internal/settings"
)

func Test词元预算缺失或者不可压缩输入不调用模型(测试器 *测试包.测试器) {
	上游, 服务端 := 压缩端点(测试器)
	启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	进程标识 := 结果标识(测试器, 当前对象.调用(测试器, "provider.create", map[string]any{"name": "未知容量", "protocol": "chatcompletions", "base_url": 服务端.URL + "/v1"}), "id")
	消息标识 := 结果标识(测试器, 当前对象.调用(测试器, "model.create", map[string]any{"provider_id": 进程标识, "display_name": "模型", "remote_model_id": "unknown", "enabled": true}), "id")
	for 索引, 类别 := range []string{"token_budget_unconfigured", "token_budget_exceeded"} {
		if 索引 == 1 {
			值 := 设置包.默认压缩()
			值.词元预算 = 100
			压缩设置(测试器, 当前对象, 值)
		}
		要求成功(测试器, 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "stream": false, "content": 消息内容块集("当前必须保留的问题" + 字符串包.重复("代码", 100))}))
		帧 := 当前对象.事件(测试器, "run.failed")
		var 载荷 struct {
			执行 智能体包.执行 `json:"run"`
		}
		if 错误 := JSON包.反序列化(帧.参数, &载荷); 错误 != nil || 载荷.执行.错误类别 == nil || *载荷.执行.错误类别 != 类别 {
			测试器.格式化报告致命错误("预算错误未明确报告: %s %v", 帧.参数, 错误)
		}
	}
	上游.互斥锁.加锁()
	数量 := len(上游.回答集)
	上游.互斥锁.解锁()
	if 数量 != 0 || 上游.摘要集.加载() != 0 {
		测试器.报告致命错误("无效预算仍调用了模型")
	}
	值 := 设置包.默认压缩()
	值.词元预算 = 50000
	压缩设置(测试器, 当前对象, 值)
	压缩发送(测试器, 当前对象, 消息标识, "", "未知容量使用显式预算")
}

func Test短历史不压缩位于旧消息阈值(测试器 *测试包.测试器) {
	// 各用例独占临时目录、运行时及受控端点，可与其他应用回归并行。
	测试器.并行()
	上游, 服务端 := 压缩端点(测试器)
	启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	消息标识 := 初始化会话模型(测试器, 当前对象, 服务端.URL)
	压缩 := 设置包.默认压缩()
	压缩.词元预算 = 100000
	压缩设置(测试器, 当前对象, 压缩)
	会话标识 := ""
	for range 65 {
		会话标识 = 压缩发送(测试器, 当前对象, 消息标识, 会话标识, "简短问题")
	}
	if 上游.摘要集.加载() != 0 || len(上下文版本集(测试器, 当前对象, 会话标识)) != 0 {
		测试器.报告致命错误("低于 Token 预算的历史仍按旧消息条数压缩")
	}
	行集 := 读取消息集(测试器, 当前对象, 会话标识)
	输入 := 步骤输入(测试器, 当前对象, 行集[len(行集)-2].执行标识)
	if !字符串包.包含(输入, `"estimation_method":"unicode-v1"`) || !字符串包.包含(输入, `"input_limit":100000`) {
		测试器.格式化报告致命错误("没有保存预算审计: %s", 输入)
	}
}

func Test实际提示词词元影响下一个工具轮次无变化中用量(测试器 *测试包.测试器) {
	for _, 流 := range []bool{false, true} {
		测试器.执行(格式化包.转为字符串(流), func(测试器 *测试包.测试器) {
			var 调用集 原子操作包.整数32
			服务端 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
				调用集.添加(1)
				if 流 {
					当前响应写入器.头部().设置("Content-Type", "text/event-stream")
					_, _ = 输入输出包.写入字符串(当前响应写入器, "data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call\",\"type\":\"function\",\"function\":{\"name\":\"missing_tool\",\"arguments\":\"{}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":20000,\"total_tokens\":20001}}\n\ndata: [DONE]\n\n")
				} else {
					当前响应写入器.头部().设置("Content-Type", "application/json")
					_, _ = 输入输出包.写入字符串(当前响应写入器, `{"choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"call","type":"function","function":{"name":"missing_tool","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":20000,"total_tokens":20001}}`)
				}
			}))
			测试器.注册清理函数(服务端.关闭)
			启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			启动配置.最大并发执行集 = 1 // 不启动并行标题，精确检查回答模型调用次数。
			当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
			消息标识 := 初始化会话模型(测试器, 当前对象, 服务端.URL)
			压缩 := 设置包.默认压缩()
			压缩.词元预算 = 10000
			压缩设置(测试器, 当前对象, 压缩)
			帧 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "stream": 流, "content": 消息内容块集("调用工具")})
			执行标识 := 结果字符串(测试器, 帧, "run_id")
			失败 := 当前对象.事件(测试器, "run.failed")
			if 调用集.加载() != 1 || !字符串包.包含(string(失败.参数), "token_budget_exceeded") {
				测试器.格式化报告致命错误("真实输入用量未影响下一轮: calls=%d %s", 调用集.加载(), 失败.参数)
			}
			已列出 := 当前对象.调用(测试器, "step.list", map[string]any{"run_id": 执行标识})
			要求成功(测试器, 已列出)
			var 步骤集 []智能体包.步骤摘要
			if 错误 := JSON包.反序列化(已列出.结果, &步骤集); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			if len(步骤集) != 2 {
				测试器.格式化报告致命错误("工具事实未保留或错误增加模型步骤: %+v", 步骤集)
			}
			用量 := 当前对象.调用(测试器, "step.data", map[string]any{"step_id": 步骤集[0].标识, "field": "usage"})
			要求成功(测试器, 用量)
			var 数据 struct {
				片段 string `json:"fragment"`
			}
			if 错误 := JSON包.反序列化(用量.结果, &数据); 错误 != nil || 数据.片段 != `{"prompt_tokens":20000,"total_tokens":20001}` {
				测试器.格式化报告致命错误("实际 usage 被估算改写: %s %v", 用量.结果, 错误)
			}
		})
	}
}

// 步骤输入 通过公开 标准输入输出 数据接口读取审计，而不是依赖测试进程中的估算器状态。
func 步骤输入(测试器 *测试包.测试器, 当前应用通信对端 *应用通信对端, 执行标识 string) string {
	测试器.标记测试辅助函数()
	帧 := 当前应用通信对端.调用(测试器, "step.list", map[string]any{"run_id": 执行标识})
	要求成功(测试器, 帧)
	var 步骤集 []智能体包.步骤摘要
	if 错误 := JSON包.反序列化(帧.结果, &步骤集); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	for _, 步骤 := range 步骤集 {
		if 步骤.类型 != "model_call" || 步骤.名称 != "生成回答" {
			continue
		}
		数据 := 当前应用通信对端.调用(测试器, "step.data", map[string]any{"step_id": 步骤.标识, "field": "input"})
		要求成功(测试器, 数据)
		var 结果 struct {
			片段 string `json:"fragment"`
		}
		if 错误 := JSON包.反序列化(数据.结果, &结果); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		return 结果.片段
	}
	测试器.报告致命错误("缺少回答模型步骤")
	return ""
}
```
