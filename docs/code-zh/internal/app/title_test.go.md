```go
package 应用模块

import (
	字节包 "bytes"
	JSON包 "encoding/json"
	输入输出包 "io"
	HTTP包 "net/http"
	HTTP测试包 "net/http/httptest"
	文件路径包 "path/filepath"
	同步包 "sync"
	测试包 "testing"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	配置模块包 "github.com/xiaoyu98628/callum/internal/config"
)

func Test手动标题优先并且不完整回答保留用量(测试器 *测试包.测试器) {
	允许标题, 允许回答 := make(chan struct{}), make(chan struct{})
	var 标题一次, 回答一次 同步包.一次
	释放标题 := func() { 标题一次.执行(func() { close(允许标题) }) }
	释放回答 := func() { 回答一次.执行(func() { close(允许回答) }) }
	测试器.注册清理函数(释放标题)
	测试器.注册清理函数(释放回答)
	上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
		var 请求 struct {
			工具集 []JSON包.原始JSON消息 `json:"tools"`
			流   bool             `json:"stream"`
			消息集 []struct {
				角色 string `json:"role"`
				内容 string `json:"content"`
			} `json:"messages"`
		}
		if 错误 := JSON包.新建解码器(当前请求.正文).解码(&请求); 错误 != nil {
			测试器.错误文本(错误)
			return
		}
		if len(请求.工具集) == 0 {
			select {
			case <-允许标题:
			case <-当前请求.上下文().完成():
				return
			}
			当前响应写入器.头部().设置("Content-Type", "application/json")
			_, _ = 输入输出包.写入字符串(当前响应写入器, `{"choices":[{"index":0,"message":{"role":"assistant","content":"不应覆盖的标题"},"finish_reason":"stop"}]}`)
			return
		}
		if 请求.消息集[len(请求.消息集)-1].内容 == "触发鉴权失败" {
			当前响应写入器.头部().设置("Content-Type", "application/json")
			当前响应写入器.头部().设置("X-Request-ID", "auth-request")
			当前响应写入器.写入头部(HTTP包.状态未授权)
			_, _ = 输入输出包.写入字符串(当前响应写入器, `{"error":{"message":"fake-dialogue-key","type":"authentication_error"}}`)
			return
		}
		if 请求.流 {
			当前响应写入器.头部().设置("Content-Type", "text/event-stream")
			_, _ = 输入输出包.写入字符串(当前响应写入器, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"正文\"},\"finish_reason\":null}]}\n\n")
			当前响应写入器.(HTTP包.刷新器).刷新()
			select {
			case <-允许回答:
			case <-当前请求.上下文().完成():
				return
			}
			_, _ = 输入输出包.写入字符串(当前响应写入器, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			return
		}
		当前响应写入器.头部().设置("Content-Type", "application/json")
		_, _ = 输入输出包.写入字符串(当前响应写入器, `{"choices":[{"index":0,"message":{"role":"assistant","content":"未完成内容"},"finish_reason":"length"}],"usage":{"total_tokens":9,"vendor":"kept"}}`)
	}))
	测试器.注册清理函数(上游.关闭)
	启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	启动配置.最大并发执行集 = 2
	当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	消息标识 := 初始化会话模型(测试器, 当前对象, 上游.URL)
	已受理 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "content": 消息内容块集("标题测试")})
	会话标识 := 结果字符串(测试器, 已受理, "conversation_id")
	当前对象.事件(测试器, "message.delta")
	要求成功(测试器, 当前对象.调用(测试器, "conversation.update", map[string]any{"id": 会话标识, "title": "人工改名"}))
	释放标题()
	for {
		帧 := 当前对象.事件(测试器, "step.updated")
		if 字节包.包含(帧.参数, []byte("生成会话标题")) && 字节包.包含(帧.参数, []byte(`"status":"completed"`)) {
			break
		}
	}
	释放回答()
	当前对象.事件(测试器, "run.completed")
	实际值 := 当前对象.调用(测试器, "conversation.get", map[string]any{"id": 会话标识})
	要求成功(测试器, 实际值)
	if !字节包.包含(实际值.结果, []byte(`"title":"人工改名"`)) || !字节包.包含(实际值.结果, []byte(`"title_source":"manual"`)) {
		测试器.格式化报告致命错误("生成标题覆盖了人工标题: %s", 实际值.结果)
	}
	失败 := 当前对象.调用(测试器, "conversation.send", map[string]any{"conversation_id": 会话标识, "stream": false, "content": 消息内容块集("触发截断")})
	执行标识 := 结果字符串(测试器, 失败, "run_id")
	当前对象.事件(测试器, "run.failed")
	行集 := 读取消息集(测试器, 当前对象, 会话标识)
	if len(行集) != 4 || 行集[3].状态 != 智能体包.失败 || 行集[3].已完成位于 != nil || 行集[3].内容[0].文本 != "未完成内容" {
		测试器.格式化报告致命错误("不完整回答被当成成功: %+v", 行集)
	}
	步骤标识 := 结果字符串来源步骤集(测试器, 当前对象.调用(测试器, "step.list", map[string]any{"run_id": 执行标识}))
	用量 := 当前对象.调用(测试器, "step.data", map[string]any{"step_id": 步骤标识, "field": "usage"})
	要求成功(测试器, 用量)
	if !字节包.包含(用量.结果, []byte("kept")) {
		测试器.格式化报告致命错误("异常结束丢失已返回 usage: %s", 用量.结果)
	}
	认证 := 当前对象.调用(测试器, "conversation.send", map[string]any{"conversation_id": 会话标识, "stream": false, "content": 消息内容块集("触发鉴权失败")})
	认证执行 := 结果字符串(测试器, 认证, "run_id")
	认证失败 := 当前对象.事件(测试器, "run.failed")
	if !字节包.包含(认证失败.参数, []byte("HTTP 401")) || 字节包.包含(认证失败.参数, []byte("fake-dialogue-key")) {
		测试器.格式化报告致命错误("鉴权错误丢失状态或泄露错误体: %s", 认证失败.参数)
	}
	认证步骤 := 结果字符串来源步骤集(测试器, 当前对象.调用(测试器, "step.list", map[string]any{"run_id": 认证执行}))
	详情 := 当前对象.调用(测试器, "step.data", map[string]any{"step_id": 认证步骤, "field": "output"})
	要求成功(测试器, 详情)
	if !字节包.包含(详情.结果, []byte("request_error")) || !字节包.包含(详情.结果, []byte("401")) || !字节包.包含(详情.结果, []byte("auth-request")) || 字节包.包含(详情.结果, []byte("fake-dialogue-key")) {
		测试器.格式化报告致命错误("未保存受控供应商错误信息: %s", 详情.结果)
	}
	当前对象.停止(测试器)
}

func 结果字符串来源步骤集(测试器 *测试包.测试器, 当前通信帧 通信帧) string {
	测试器.标记测试辅助函数()
	要求成功(测试器, 当前通信帧)
	var 行集 []struct {
		标识 string `json:"id"`
	}
	if 错误 := JSON包.反序列化(当前通信帧.结果, &行集); 错误 != nil || len(行集) != 1 {
		测试器.格式化报告致命错误("步骤数量不正确: %s", 当前通信帧.结果)
	}
	return 行集[0].标识
}
```
