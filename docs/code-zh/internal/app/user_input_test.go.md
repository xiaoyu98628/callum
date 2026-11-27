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
	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

func Test用户输入通过标准输入输出(测试器 *测试包.测试器) {
	for _, 模式 := range []string{"option", "text", "skip", "cancel", "close", "invalid"} {
		测试器.执行(模式, func(测试器 *测试包.测试器) {
			var 回复集 原子操作包.整数32
			上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
				var 请求 struct {
					流   bool `json:"stream"`
					工具集 []struct {
						函数 模型包.工具定义 `json:"function"`
					} `json:"tools"`
					消息集 []struct {
						角色 string `json:"role"`
						内容 string `json:"content"`
					} `json:"messages"`
				}
				if 错误 := JSON包.新建解码器(当前请求.正文).解码(&请求); 错误 != nil {
					测试器.错误文本(错误)
					当前响应写入器.写入头部(400)
					return
				}
				if !请求.流 {
					当前响应写入器.头部().设置("Content-Type", "application/json")
					格式化包.输出文本(当前响应写入器, `{"choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"stop"}]}`)
					return
				}
				已找到询问 := false
				for _, 定义 := range 请求.工具集 {
					if 定义.函数.名称 == 智能体包.用户输入工具 {
						已找到询问 = 字符串包.包含(定义.函数.说明, "2-3 mutually exclusive choices") &&
							!字符串包.包含(字符串包.目标小写(定义.函数.说明), "codex")
					}
				}
				if !已找到询问 {
					测试器.错误文本("模型未收到无品牌字样的互斥选项指引")
				}
				当前响应写入器.头部().设置("Content-Type", "text/event-stream")
				最后 := 请求.消息集[len(请求.消息集)-1]
				if 最后.角色 == "tool" || 最后.内容 == "继续对话" {
					回复集.添加(1)
					格式化包.输出文本(当前响应写入器, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"已收到回复\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
					return
				}
				参数 := `{"question":"另一个任务是否已结束？","options":["已结束，可以复验","仍在进行"]}`
				if 模式 == "invalid" {
					参数 = `{"question":"问题","options":["重复","重复"]}`
				}
				数据, _ := JSON包.序列化(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "ask-call", "type": "function", "function": map[string]string{"name": 智能体包.用户输入工具, "arguments": 参数}}}}, "finish_reason": nil}}})
				格式化包.格式化输出(当前响应写入器, "data: %s\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n", 数据)
			}))
			测试器.注册清理函数(上游.关闭)
			启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
			模型标识 := 初始化会话模型(测试器, 当前对象, 上游.URL)
			已受理 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 模型标识, "stream": true, "content": 消息内容块集("请确认任务")})
			执行标识 := 结果字符串(测试器, 已受理, "run_id")
			会话标识 := 结果字符串(测试器, 已受理, "conversation_id")
			if 模式 == "invalid" {
				当前对象.事件(测试器, "run.completed")
				要求成功(测试器, 当前对象.调用(测试器, "conversation.send", map[string]any{"conversation_id": 会话标识, "stream": true, "content": 消息内容块集("继续")}))
				当前对象.事件(测试器, "run.completed")
				当前对象.停止(测试器)
				return
			}
			帧 := 当前对象.事件(测试器, "user_input.requested")
			var 载荷 struct {
				输入   智能体包.用户输入 `json:"user_input"`
				执行标识 string    `json:"run_id"`
				输入标识 string    `json:"input_id"`
			}
			if 错误 := JSON包.反序列化(帧.参数, &载荷); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			if 载荷.执行标识 != 执行标识 || 载荷.输入.标识 != 载荷.输入标识 || 载荷.输入.状态 != "pending" || len(载荷.输入.选项集) != 2 {
				测试器.格式化报告致命错误("问题归属或快照错误 %s", 帧.参数)
			}
			if 回复集.加载() != 0 {
				测试器.报告致命错误("用户回复前已继续模型调用")
			}
			待处理 := 当前对象.调用(测试器, "user_input.list", map[string]any{"run_id": 执行标识, "status": "pending"})
			要求成功(测试器, 待处理)
			if !字符串包.包含(string(待处理.结果), 载荷.输入标识) {
				测试器.报告致命错误("查询未恢复等待问题")
			}
			if 模式 == "cancel" || 模式 == "close" {
				if 模式 == "cancel" {
					要求成功(测试器, 当前对象.调用(测试器, "run.cancel", map[string]any{"run_id": 执行标识}))
					当前对象.事件(测试器, "run.cancelled")
				} else {
					当前对象.停止(测试器)
					当前对象 = 开始应用(测试器, 启动配置, 输入输出包.丢弃)
				}
				当前 := 当前对象.调用(测试器, "user_input.get", map[string]any{"id": 载荷.输入标识})
				要求成功(测试器, 当前)
				if !字符串包.包含(string(当前.结果), `"status":"cancelled"`) {
					测试器.格式化报告致命错误("取消问题未保存 %s", 当前.结果)
				}
				if 回复集.加载() != 0 {
					测试器.报告致命错误("取消后继续调用模型")
				}
				当前对象.停止(测试器)
				return
			}
			回答 := 智能体包.输入回答{}
			switch 模式 {
			case "option":
				回答.选项索引 = new(0)
			case "text":
				回答.文本 = "自己的补充回复"
			case "skip":
				回答.已跳过 = true
			}
			if 已拒绝 := 当前对象.调用(测试器, "user_input.submit", map[string]any{"input_id": 载荷.输入标识, "run_id": 执行标识, "answer": 智能体包.输入回答{选项索引: new(99)}}); 已拒绝.错误 == nil {
				测试器.报告致命错误("接受越界选项")
			}
			其他 := 执行标识[:len(执行标识)-1] + "f"
			if 其他 == 执行标识 {
				其他 = 执行标识[:len(执行标识)-1] + "e"
			}
			if 已拒绝 := 当前对象.调用(测试器, "user_input.submit", map[string]any{"input_id": 载荷.输入标识, "run_id": 其他, "answer": 回答}); 已拒绝.错误 == nil {
				测试器.报告致命错误("接受跨执行回复")
			}
			参数 := map[string]any{"input_id": 载荷.输入标识, "run_id": 执行标识, "answer": 回答}
			要求成功(测试器, 当前对象.调用(测试器, "user_input.submit", 参数))
			要求成功(测试器, 当前对象.调用(测试器, "user_input.submit", 参数))
			if 已拒绝 := 当前对象.调用(测试器, "user_input.submit", map[string]any{"input_id": 载荷.输入标识, "run_id": 执行标识, "answer": 智能体包.输入回答{文本: "冲突改答"}}); 已拒绝.错误 == nil {
				测试器.报告致命错误("覆盖了已保存答案")
			}
			当前对象.事件(测试器, "run.completed")
			if 回复集.加载() != 1 {
				测试器.报告致命错误("回复重复触发模型调用", 回复集.加载())
			}
			当前对象.停止(测试器)
			当前对象 = 开始应用(测试器, 启动配置, 输入输出包.丢弃)
			当前 := 当前对象.调用(测试器, "user_input.get", map[string]any{"id": 载荷.输入标识})
			要求成功(测试器, 当前)
			预期 := "answered"
			if 模式 == "skip" {
				预期 = "skipped"
			}
			if !字符串包.包含(string(当前.结果), `"status":"`+预期+`"`) {
				测试器.格式化报告致命错误("重启丢失回复 %s", 当前.结果)
			}
			要求成功(测试器, 当前对象.调用(测试器, "conversation.send", map[string]any{"conversation_id": 会话标识, "stream": true, "content": 消息内容块集("继续对话")}))
			当前对象.事件(测试器, "run.completed")
			当前对象.停止(测试器)
		})
	}
}
```
