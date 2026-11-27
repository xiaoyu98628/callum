```go
package 应用模块

import (
	字节包 "bytes"
	上下文包 "context"
	JSON包 "encoding/json"
	格式化包 "fmt"
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
	事件集包 "github.com/xiaoyu98628/callum/internal/agent/events"
	配置模块包 "github.com/xiaoyu98628/callum/internal/config"
	历史包 "github.com/xiaoyu98628/callum/internal/conversation/history"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

func Test智能体工具集并且审批通过标准输入输出(测试器 *测试包.测试器) {
	const 前导 = "我会先读取文件，确认内容后再回答。"
	for _, 模式 := range []string{"automatic", "never", "approve", "reject", "cancel"} {
		测试器.执行(模式, func(测试器 *测试包.测试器) {
			自动 := 模式 == "automatic" || 模式 == "never"
			var 工具结果集 原子操作包.整数32
			上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
				var 输入 struct {
					流   bool             `json:"stream"`
					工具集 []JSON包.原始JSON消息 `json:"tools"`
					消息集 []struct {
						角色     string           `json:"role"`
						内容     string           `json:"content"`
						工具调用标识 string           `json:"tool_call_id"`
						调用集    []JSON包.原始JSON消息 `json:"tool_calls"`
					} `json:"messages"`
				}
				if 错误 := JSON包.新建解码器(当前请求.正文).解码(&输入); 错误 != nil {
					测试器.错误文本(错误)
					当前响应写入器.写入头部(400)
					return
				}
				if len(输入.工具集) != 9 {
					测试器.格式化创建错误("未提供完整的内置工具定义: %d", len(输入.工具集))
				}
				if len(输入.消息集) < 2 || 输入.消息集[0].角色 != "system" || !字符串包.包含(输入.消息集[0].内容, "1～2句话") || !字符串包.包含(输入.消息集[0].内容, "正文 content") {
					测试器.错误文本("主对话未发送操作前说明的固定提示")
				} else if !字符串包.存在前缀(输入.消息集[0].内容, "You are Callum,") || !字符串包.包含(输入.消息集[0].内容, "# Autonomy and persistence") || 字符串包.包含(字符串包.目标小写(输入.消息集[0].内容), "codex") {
					测试器.错误文本("主对话未使用适配后的身份和任务推进指引")
				}
				for _, 消息 := range 输入.消息集[1:] {
					if 消息.角色 == "system" {
						测试器.错误文本("多轮工具上下文重复累计了固定系统提示")
					}
				}
				最后 := 输入.消息集[len(输入.消息集)-1]
				if 最后.角色 == "tool" {
					工具结果集.添加(1)
					if 最后.工具调用标识 != "call-read" || len(输入.消息集) < 3 || len(输入.消息集[len(输入.消息集)-2].调用集) != 1 {
						测试器.错误文本("工具调用与结果未配对")
					}
					if 自动 || 模式 == "approve" {
						if !字节包.包含([]byte(最后.内容), []byte("workspace-content")) {
							测试器.格式化创建错误("未读取工作区文件: %s", 最后.内容)
						}
					} else if !字节包.包含([]byte(最后.内容), []byte("error")) {
						测试器.格式化创建错误("拒绝未返回结构化结果: %s", 最后.内容)
					}
					if 输入.流 {
						当前响应写入器.头部().设置("Content-Type", "text/event-stream")
						_, _ = 输入输出包.写入字符串(当前响应写入器, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"最终回答\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"total_tokens\":7}}\n\ndata: [DONE]\n\n")
					} else {
						当前响应写入器.头部().设置("Content-Type", "application/json")
						_, _ = 输入输出包.写入字符串(当前响应写入器, `{"choices":[{"index":0,"message":{"role":"assistant","content":"最终回答"},"finish_reason":"stop"}],"usage":{"total_tokens":7}}`)
					}
					return
				}
				// 后续用户问题必须带上先前的工具轨迹；直接回答即可。
				if 最后.内容 == "继续" {
					已找到 := false
					for _, 映射 := range 输入.消息集 {
						if 映射.角色 == "tool" {
							已找到 = true
						}
					}
					if !已找到 {
						测试器.错误文本("后续上下文丢失工具轨迹")
					}
					当前响应写入器.头部().设置("Content-Type", "application/json")
					_, _ = 输入输出包.写入字符串(当前响应写入器, `{"choices":[{"index":0,"message":{"role":"assistant","content":"后续回答"},"finish_reason":"stop"}]}`)
					return
				}
				if 输入.流 {
					当前响应写入器.头部().设置("Content-Type", "text/event-stream")
					_, _ = 格式化包.格式化输出(当前响应写入器, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":%q},\"finish_reason\":null}]}\n\n", 前导)
					_, _ = 输入输出包.写入字符串(当前响应写入器, "data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-read\",\"type\":\"function\",\"function\":{\"name\":\"read_file\",\"arguments\":\"{\\\"path\\\":\"}}]},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"a.txt\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"total_tokens\":3,\"vendor\":true}}\n\ndata: [DONE]\n\n")
				} else {
					当前响应写入器.头部().设置("Content-Type", "application/json")
					_, _ = 输入输出包.写入字符串(当前响应写入器, `{"choices":[{"index":0,"message":{"role":"assistant","content":"我会先读取文件，确认内容后再回答。","tool_calls":[{"id":"call-read","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"a.txt\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"total_tokens":3,"vendor":true}}`)
				}
			}))
			测试器.注册清理函数(上游.关闭)
			启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			启动配置.最大并发执行集 = 1
			if 模式 == "approve" || 模式 == "cancel" {
				启动配置.调用超时 = 250 * 时间包.毫秒
			}
			当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
			if 模式 == "never" {
				设置审批策略(测试器, 当前对象, "never")
			} else if !自动 {
				设置审批策略(测试器, 当前对象, "always")
			}
			消息标识 := 初始化会话模型(测试器, 当前对象, 上游.URL)
			根 := 测试器.创建临时目录()
			if 错误 := 操作系统包.写入文件(文件路径包.合并(根, "a.txt"), []byte("workspace-content"), 0600); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			工作区标识 := 结果字符串(测试器, 当前对象.调用(测试器, "workspace.create", map[string]any{"name": "项目", "root_path": 根}), "id")
			流 := 模式 == "automatic" || 模式 == "approve"
			已受理 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "workspace_id": 工作区标识, "stream": 流, "content": 消息内容块集("读取文件")})
			执行标识, 会话标识 := 结果字符串(测试器, 已受理, "run_id"), 结果字符串(测试器, 已受理, "conversation_id")
			var 审批标识 string
			if !自动 {
				通知 := 当前对象.事件(测试器, "approval.requested")
				var 载荷 struct {
					审批标识 string  `json:"approval_id"`
					会话标识 string  `json:"conversation_id"`
					审批   智能体包.审批 `json:"approval"`
				}
				if 错误 := JSON包.反序列化(通知.参数, &载荷); 错误 != nil {
					测试器.报告致命错误(错误)
				}
				if 字节包.包含(通知.参数, []byte(`"expires_at"`)) {
					测试器.报告致命错误("审批通知仍带过期字段")
				}
				审批标识 = 载荷.审批标识
				if 载荷.会话标识 != 会话标识 {
					测试器.报告致命错误("审批事件丢失会话标识")
				}
				if 载荷.审批.请求哈希 == "" || !字节包.包含(载荷.审批.请求, []byte("read_file")) {
					测试器.报告致命错误("未保存确定操作快照")
				}
				if 工具结果集.加载() != 0 {
					测试器.报告致命错误("审批前已经执行工具")
				}
				if 模式 == "approve" || 模式 == "cancel" {
					// 暂不作决定并等待超过单次模型超时，审批应保持 待处理，之后仍可批准或取消。
					计时器 := 时间包.新建计时器(350 * 时间包.毫秒)
					select {
					case <-计时器.字符:
					case <-测试器.上下文().完成():
						测试器.报告致命错误(测试器.上下文().获取错误())
					}
					计时器.停止()
					当前 := 当前对象.调用(测试器, "approval.get", map[string]any{"id": 审批标识})
					要求成功(测试器, 当前)
					var 等待中 智能体包.审批
					if 错误 := JSON包.反序列化(当前.结果, &等待中); 错误 != nil || 等待中.状态 != 智能体包.审批待处理 || 等待中.已决定位于 != nil {
						测试器.格式化报告致命错误("等待中的审批提前结束: %s %v", 当前.结果, 错误)
					}
					if 工具结果集.加载() != 0 {
						测试器.报告致命错误("等待期间执行了未批准操作")
					}
				}
				if 模式 == "approve" || 模式 == "reject" {
					决定 := 智能体包.审批已批准
					if 模式 == "reject" {
						决定 = 智能体包.审批已拒绝
					}
					参数 := map[string]any{"approval_id": 审批标识, "decision": 决定, "decision_note": "用户说明"}
					要求成功(测试器, 当前对象.调用(测试器, "approval.decide", 参数))
					要求成功(测试器, 当前对象.调用(测试器, "approval.decide", 参数))
					相反 := 智能体包.审批已批准
					if 决定 == 智能体包.审批已批准 {
						相反 = 智能体包.审批已拒绝
					}
					if 结果 := 当前对象.调用(测试器, "approval.decide", map[string]any{"approval_id": 审批标识, "decision": 相反}); 结果.错误 == nil {
						测试器.报告致命错误("允许冲突改判")
					}
				}
				if 模式 == "cancel" {
					要求成功(测试器, 当前对象.调用(测试器, "run.cancel", map[string]any{"run_id": 执行标识}))
				}
			}
			终态 := "run.completed"
			if 模式 == "cancel" {
				终态 = "run.cancelled"
			}
			当前对象.事件(测试器, 终态)
			已见 := 当前对象.已见.字符串()
			if 介绍, 工具 := 字符串包.索引(已见, 前导), 字符串包.索引(已见, `"type":"tool_call"`); 介绍 < 0 || 工具 < 介绍 {
				测试器.报告致命错误("执行前说明未在工具步骤之前发布")
			}
			if 模式 == "cancel" {
				if 工具结果集.加载() != 0 {
					测试器.报告致命错误("取消后执行了待审批工具")
				}
			} else if 工具结果集.加载() != 1 {
				测试器.格式化报告致命错误("工具结果次数错误: %d", 工具结果集.加载())
			}
			审批集 := 当前对象.调用(测试器, "approval.list", map[string]any{"run_id": 执行标识})
			要求成功(测试器, 审批集)
			var 记录集 []智能体包.审批
			if 错误 := JSON包.反序列化(审批集.结果, &记录集); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			if 自动 {
				if len(记录集) != 0 {
					测试器.报告致命错误("自动放行伪造了用户审批")
				}
			} else {
				预期 := map[string]string{"approve": 智能体包.审批已批准, "reject": 智能体包.审批已拒绝, "cancel": 智能体包.审批已取消}[模式]
				if len(记录集) != 1 || 记录集[0].标识 != 审批标识 || 记录集[0].状态 != 预期 || 记录集[0].已决定位于 == nil {
					测试器.格式化报告致命错误("审批终态错误: %+v", 记录集)
				}
			}
			步骤集帧 := 当前对象.调用(测试器, "step.list", map[string]any{"run_id": 执行标识})
			要求成功(测试器, 步骤集帧)
			var 步骤集 []智能体包.步骤摘要
			if 错误 := JSON包.反序列化(步骤集帧.结果, &步骤集); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			模型调用集, 工具调用集 := 0, 0
			for _, 步骤 := range 步骤集 {
				if 步骤.类型 == "model_call" {
					模型调用集++
					if 步骤.用量字节 == 0 {
						测试器.报告致命错误("模型步骤丢失 usage")
					}
					if 模型调用集 == 1 {
						数据 := 当前对象.调用(测试器, "step.data", map[string]any{"step_id": 步骤.标识, "field": "output"})
						要求成功(测试器, 数据)
						var 页 struct {
							片段 string
							完成 bool
						}
						if 错误 := JSON包.反序列化(数据.结果, &页); 错误 != nil || !页.完成 {
							测试器.格式化报告致命错误("模型说明读取不完整: %s %v", 数据.结果, 错误)
						}
						var 已保存 struct {
							消息 模型包.消息 `json:"context_message"`
						}
						if 错误 := JSON包.反序列化([]byte(页.片段), &已保存); 错误 != nil || 已保存.消息.内容 != 前导 || len(已保存.消息.工具调用集) != 1 {
							测试器.格式化报告致命错误("模型说明与工具调用未一起保存: %+v %v", 已保存, 错误)
						}
					}
				}
				if 步骤.类型 == "tool_call" {
					工具调用集++
					if 步骤.父级步骤标识 == nil {
						测试器.报告致命错误("工具步骤无父模型步骤")
					}
				}
			}
			if 工具调用集 != 1 || 模型调用集 != map[bool]int{true: 1, false: 2}[模式 == "cancel"] {
				测试器.格式化报告致命错误("步骤记录错误: %s", 步骤集帧.结果)
			}
			if 模式 != "cancel" {
				行集 := 读取消息集(测试器, 当前对象, 会话标识)
				if len(行集) != 2 || len(行集[1].内容) != 1 || 行集[1].内容[0].文本 != "最终回答" {
					测试器.报告致命错误("过程说明混入最终回答或产生额外消息")
				}
			}
			if 模式 == "automatic" {
				已受理 := 当前对象.调用(测试器, "conversation.send", map[string]any{"conversation_id": 会话标识, "stream": false, "content": 消息内容块集("继续")})
				要求成功(测试器, 已受理)
				当前对象.事件(测试器, "run.completed")
				if 行集 := 读取消息集(测试器, 当前对象, 会话标识); len(行集) != 4 {
					测试器.格式化报告致命错误("消息与工具轨迹混存: %+v", 行集)
				}
			}
			当前对象.停止(测试器)
		})
	}
}

func Test智能体完成超出前一个调用上限集(测试器 *测试包.测试器) {
	测试器.并行()
	for _, 流 := range []bool{false, true} {
		测试器.执行(格式化包.转为字符串(流), func(测试器 *测试包.测试器) {
			测试器.并行()
			var 请求集 原子操作包.整数32
			上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
				轮次 := int(请求集.添加(1))
				var 输入 struct {
					消息集 []struct {
						角色, 内容 string
						工具调用标识 string `json:"tool_call_id"`
					} `json:"messages"`
				}
				if 错误 := JSON包.新建解码器(当前请求.正文).解码(&输入); 错误 != nil {
					测试器.错误文本(错误)
					当前响应写入器.写入头部(400)
					return
				}
				结果集 := 0
				for _, 消息 := range 输入.消息集 {
					if 消息.角色 != "tool" {
						continue
					}
					结果集++
					if 消息.工具调用标识 == "" || !字符串包.包含(消息.内容, "workspace-content") {
						测试器.错误文本("实际文件读取结果未完整回传模型")
					}
				}
				if 最后 := 输入.消息集[len(输入.消息集)-1]; 最后.角色 == "user" && 最后.内容 == "继续任务" {
					if 结果集 != 48 {
						测试器.格式化创建错误("后续请求未恢复完整工具历史: %d", 结果集)
					}
					写入文件工具响应(测试器, 当前响应写入器, 流, map[string]any{"content": "已恢复全部工具历史"}, "stop")
					return
				}
				预期 := 0
				if 轮次 > 1 {
					预期 = 40 + 轮次 - 2
				}
				if 结果集 != 预期 {
					测试器.格式化创建错误("工具轨迹缺失: round=%d got=%d want=%d", 轮次, 结果集, 预期)
				}
				if 轮次 == 10 {
					写入文件工具响应(测试器, 当前响应写入器, 流, map[string]any{"content": "全部工具调用完成"}, "stop")
					return
				}
				数量 := 1
				if 轮次 == 1 {
					数量 = 40
				}
				调用集 := make([]any, 数量)
				for 索引 := range 数量 {
					调用 := map[string]any{"id": 格式化包.格式化字符串("read-%d-%d", 轮次, 索引), "type": "function", "function": map[string]string{"name": "read_file", "arguments": `{"path":"a.txt"}`}}
					if 流 {
						调用["index"] = 索引
					}
					调用集[索引] = 调用
				}
				写入文件工具响应(测试器, 当前响应写入器, 流, map[string]any{"tool_calls": 调用集}, "tool_calls")
			}))
			测试器.注册清理函数(上游.关闭)
			启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			启动配置.最大并发执行集 = 1
			当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
			消息标识 := 初始化会话模型(测试器, 当前对象, 上游.URL)
			根 := 测试器.创建临时目录()
			if 错误 := 操作系统包.写入文件(文件路径包.合并(根, "a.txt"), []byte("workspace-content"), 0600); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			工作区标识 := 结果字符串(测试器, 当前对象.调用(测试器, "workspace.create", map[string]any{"name": "项目", "root_path": 根}), "id")
			响应 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "workspace_id": 工作区标识, "stream": 流, "content": 消息内容块集("多轮读取并回答")})
			执行标识 := 结果字符串(测试器, 响应, "run_id")
			当前对象.事件(测试器, "run.completed")
			帧 := 当前对象.调用(测试器, "step.list", map[string]any{"run_id": 执行标识, "limit": 200})
			要求成功(测试器, 帧)
			var 步骤集 []智能体包.步骤摘要
			if 错误 := JSON包.反序列化(帧.结果, &步骤集); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			模型集, 工具集 := 0, 0
			for _, 步骤 := range 步骤集 {
				if 步骤.状态 != 智能体包.已完成 {
					测试器.格式化报告致命错误("步骤未成功: %+v", 步骤)
				}
				if 步骤.类型 == "model_call" {
					模型集++
					if 步骤.用量字节 == 0 {
						测试器.报告致命错误("模型实际 usage 丢失")
					}
				}
				if 步骤.类型 == "tool_call" {
					工具集++
					if 步骤.父级步骤标识 == nil || 步骤.输出字节 == 0 {
						测试器.报告致命错误("工具父步骤或结果丢失")
					}
				}
			}
			if 请求集.加载() != 10 || 模型集 != 10 || 工具集 != 48 {
				测试器.格式化报告致命错误("长调用链未完整完成: requests=%d models=%d tools=%d", 请求集.加载(), 模型集, 工具集)
			}
			要求成功(测试器, 当前对象.调用(测试器, "conversation.send", map[string]any{"conversation_id": 结果字符串(测试器, 响应, "conversation_id"), "stream": 流, "content": 消息内容块集("继续任务")}))
			当前对象.事件(测试器, "run.completed")
			if 请求集.加载() != 11 {
				测试器.报告致命错误("后续请求重复执行了历史工具调用")
			}
			当前对象.停止(测试器)
		})
	}
}

func Test智能体检查集大型工具批次之前执行中(测试器 *测试包.测试器) {
	测试器.并行()
	for _, 边界 := range []string{"bytes", "messages"} {
		测试器.执行(边界, func(测试器 *测试包.测试器) {
			测试器.并行()
			参数, 数量 := `{}`, 300
			if 边界 == "bytes" {
				参数, 数量 = `{"path":"`+字符串包.重复("x", 15900)+`"}`, 526
			}
			调用集 := make([]模型包.工具调用, 数量)
			for 索引 := range 调用集 {
				调用集[索引] = 模型包.工具调用{标识: 格式化包.格式化字符串("call-%d", 索引), 名称: "operation", 参数: []byte(参数)}
			}
			已编码, 错误 := JSON包.序列化([]模型包.消息{{角色: 模型包.角色助手, 工具调用集: 调用集}})
			if 错误 != nil || (边界 == "bytes" && len(已编码) <= 历史包.最大字节) {
				测试器.格式化报告致命错误("测试输入未越过上下文边界: %d %v", len(已编码), 错误)
			}
			运行时, _ := 运行时测试夹具(测试器, 1)
			实例 := &操作结果测试工具{路径: 文件路径包.合并(测试器.创建临时目录(), "evidence"), 操作结果: 工具包.操作结果已生效}
			var 请求集 原子操作包.整数32
			客户端 := 操作结果测试模型(func(上下文包.上下文, 模型包.请求) (模型包.响应, error) {
				请求集.添加(1)
				return 模型包.响应{工具调用集: 调用集, 输出: []byte(`{"fixture":true}`)}, nil
			})
			标识 := 策略测试标识(测试器)
			服务, _ := 操作结果服务(测试器, 运行时, 实例, 客户端, 标识, nil)
			订阅, 错误 := 运行时.订阅(事件集包.筛选{})
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			defer 订阅.关闭()
			已受理 := 发送操作结果(测试器, 服务, 标识, "", "检查上下文边界", nil)
			执行上下文, 取消函数 := 上下文包.创建超时上下文(测试器.上下文(), 5*时间包.秒)
			defer 取消函数()
			已创建, 已保留 := 0, false
			for {
				事件, 错误 := 订阅.下一个(执行上下文)
				if 错误 != nil {
					测试器.报告致命错误(错误)
				}
				if 事件.执行标识 != 已受理.执行.标识 {
					continue
				}
				switch 事件.类型 {
				case "step.created":
					步骤 := 事件.数据.(智能体包.步骤摘要)
					已创建++
					if 步骤.类型 != "model_call" {
						测试器.报告致命错误("越界检查前已受理工具操作")
					}
				case "step.updated":
					步骤 := 事件.数据.(智能体包.步骤摘要)
					// 更新事件由实际保存成功后发布；避免通过列表重新解码整份超大结果。
					已保留 = 已保留 || (步骤.类型 == "model_call" && 步骤.状态 == 智能体包.已完成 && 步骤.输出字节 > 0)
				case "run.completed", "run.cancelled":
					测试器.格式化报告致命错误("越界调用被当作成功或取消: %s", 事件.类型)
				case "run.failed":
					运行 := 事件.数据.(智能体包.执行)
					if 运行.错误类别 == nil || *运行.错误类别 != "input_limit" {
						测试器.格式化报告致命错误("没有按上下文边界拒绝: %+v", 运行)
					}
					运行时.管理器.等待(已受理.执行.标识)
					if 已创建 != 1 || !已保留 || 请求集.加载() != 1 || 实例.调用集.加载() != 0 {
						测试器.报告致命错误("越界前执行了工具，或模型结果未保存")
					}
					return
				}
			}
		})
	}
}
```
