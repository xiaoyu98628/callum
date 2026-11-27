```go
package 应用模块

import (
	字节包 "bytes"
	JSON包 "encoding/json"
	格式化包 "fmt"
	输入输出包 "io"
	HTTP包 "net/http"
	HTTP测试包 "net/http/httptest"
	文件路径包 "path/filepath"
	切片包 "slices"
	字符串包 "strings"
	同步包 "sync"
	测试包 "testing"

	UUID包 "github.com/google/uuid"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	配置模块包 "github.com/xiaoyu98628/callum/internal/config"
	会话包 "github.com/xiaoyu98628/callum/internal/conversation"
	SQLite包 "github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

func 结果字符串(测试器 *测试包.测试器, 当前通信帧 通信帧, 键 string) string {
	测试器.标记测试辅助函数()
	要求成功(测试器, 当前通信帧)
	var 值集 map[string]JSON包.原始JSON消息
	if 错误 := JSON包.反序列化(当前通信帧.结果, &值集); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	var 值 string
	if 错误 := JSON包.反序列化(值集[键], &值); 错误 != nil || 值 == "" {
		测试器.格式化报告致命错误("缺少 %s: %s", 键, 当前通信帧.结果)
	}
	return 值
}

func 初始化会话模型(测试器 *测试包.测试器, 当前应用通信对端 *应用通信对端, URL string) string {
	测试器.标记测试辅助函数()
	进程标识 := 结果标识(测试器, 当前应用通信对端.调用(测试器, "provider.create", map[string]any{"name": "测试厂商", "protocol": "chatcompletions", "base_url": URL + "/v1", "api_key": "fake-dialogue-key"}), "id")
	return 结果标识(测试器, 当前应用通信对端.调用(测试器, "model.create", map[string]any{"provider_id": 进程标识, "display_name": "测试模型", "remote_model_id": "remote-model", "enabled": true, "context_window": 1000000}), "id")
}

func 消息内容块集(文本 string) []map[string]string {
	return []map[string]string{{"type": "text", "text": 文本}}
}

func 读取消息集(测试器 *测试包.测试器, 当前应用通信对端 *应用通信对端, 标识 string) []会话包.消息 {
	测试器.标记测试辅助函数()
	var 行集 []会话包.消息
	之前 := ""
	for {
		帧 := 当前应用通信对端.调用(测试器, "conversation.history", map[string]any{"conversation_id": 标识, "before_run_id": 之前, "limit": 25})
		要求成功(测试器, 帧)
		var 页 会话包.历史页
		if 错误 := JSON包.反序列化(帧.结果, &页); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		行集 = append(页.消息集, 行集...)
		// 独立压缩页可能没有消息；只按服务端结束标记停止，并拒绝不推进的游标。
		if !页.还有更多 {
			return 行集
		}
		if 页.之前执行标识 == "" || (之前 != "" && 页.之前执行标识 >= 之前) {
			测试器.格式化报告致命错误("历史分页游标未推进: %q -> %q", 之前, 页.之前执行标识)
		}
		之前 = 页.之前执行标识
	}
}

func Test会话消息集方法是不可用(测试器 *测试包.测试器) {
	测试器.并行()
	启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	帧 := 当前对象.调用(测试器, "conversation.messages", map[string]any{"conversation_id": "019a4f39-0000-7000-8000-000000000001"})
	if 帧.错误 == nil || 帧.错误.代码 != -32601 {
		测试器.格式化报告致命错误("已移除接口未返回 method not found: %+v", 帧.错误)
	}
	当前对象.停止(测试器)
}

func Test会话生命周期通过标准输入输出(测试器 *测试包.测试器) {
	结束首个 := make(chan struct{})
	var 一次 同步包.一次
	释放首个 := func() { 一次.执行(func() { close(结束首个) }) }
	测试器.注册清理函数(释放首个)
	var 互斥锁 同步包.互斥锁
	历史集 := map[string][]string{}
	上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
		if 当前请求.头部.获取("Authorization") != "Bearer fake-dialogue-key" {
			测试器.错误文本("未使用配置快照中的测试凭据")
			当前响应写入器.写入头部(401)
			return
		}
		var 输入 struct {
			工具集 []JSON包.原始JSON消息 `json:"tools"`
			流   bool             `json:"stream"`
			消息集 []struct {
				角色 string `json:"role"`
				内容 string `json:"content"`
			} `json:"messages"`
		}
		if 错误 := JSON包.新建解码器(当前请求.正文).解码(&输入); 错误 != nil || len(输入.消息集) == 0 {
			测试器.格式化创建错误("请求错误: %v", 错误)
			当前响应写入器.写入头部(400)
			return
		}
		if len(输入.工具集) == 0 {
			当前响应写入器.头部().设置("Content-Type", "application/json")
			当前响应写入器.头部().设置("X-Request-ID", "title-request")
			_, _ = 输入输出包.写入字符串(当前响应写入器, `{"choices":[{"index":0,"message":{"role":"assistant","content":"模型生成的标题"},"finish_reason":"stop"}],"usage":{"total_tokens":3,"custom":{"title":true}}}`)
			return
		}
		文本 := 输入.消息集[len(输入.消息集)-1].内容
		互斥锁.加锁()
		// 固定上下文可以增加元数据，持久化历史仍须按原文和顺序恢复。
		for _, 消息 := range 输入.消息集 {
			if 消息.角色 != "system" {
				历史集[文本] = append(历史集[文本], 消息.内容)
			}
		}
		互斥锁.解锁()
		当前响应写入器.头部().设置("X-Request-ID", "answer-request")
		if 输入.流 {
			当前响应写入器.头部().设置("Content-Type", "text/event-stream")
			_, _ = 输入输出包.写入字符串(当前响应写入器, "data: {\"id\":\"response\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"流式正文\"},\"finish_reason\":null}]}\n\n")
			当前响应写入器.(HTTP包.刷新器).刷新()
			select {
			case <-结束首个:
			case <-当前请求.上下文().完成():
				return
			}
			_, _ = 输入输出包.写入字符串(当前响应写入器, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"total_tokens\":11,\"custom\":{\"answer\":true}}}\n\ndata: [DONE]\n\n")
			return
		}
		当前响应写入器.头部().设置("Content-Type", "application/json")
		已编码, _ := JSON包.序列化("回答：" + 文本)
		_, _ = 格式化包.格式化输出(当前响应写入器, `{"provider_extra":"kept","choices":[{"index":0,"message":{"role":"assistant","content":%s},"finish_reason":"stop"}],"usage":{"total_tokens":5}}`, 已编码)
	}))
	测试器.注册清理函数(上游.关闭)
	启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	启动配置.最大并发执行集 = 2
	var 日志集 字节包.缓冲区
	当前对象 := 开始应用(测试器, 启动配置, &日志集)
	模型标识 := 初始化会话模型(测试器, 当前对象, 上游.URL)
	问题 := "  请帮我 review this project 的结构和代码并说明如何调整  "
	已受理 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 模型标识, "content": 消息内容块集(问题)})
	会话标识, 执行标识, 用户标识 := 结果字符串(测试器, 已受理, "conversation_id"), 结果字符串(测试器, 已受理, "run_id"), 结果字符串(测试器, 已受理, "user_message_id")
	if 标识, 错误 := UUID包.解析(执行标识); 错误 != nil || 标识.版本() != 7 {
		测试器.格式化报告致命错误("执行不是 UUIDv7: %s", 执行标识)
	}
	已创建 := 当前对象.事件(测试器, "conversation.created")
	var 数据 struct {
		会话 会话包.会话 `json:"conversation"`
	}
	if 错误 := JSON包.反序列化(已创建.参数, &数据); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	预期 := string([]rune(字符串包.裁剪空间(问题))[:30])
	if 数据.会话.标题 != 预期 || 数据.会话.标题来源 != "default" {
		测试器.格式化报告致命错误("默认标题不正确: %+v", 数据.会话)
	}
	增量 := 当前对象.事件(测试器, "message.delta")
	if !字节包.包含(增量.参数, []byte(执行标识)) || !字节包.包含(增量.参数, []byte(`"step_id"`)) || 字节包.包含(增量.参数, []byte("模型生成的标题")) {
		测试器.格式化报告致命错误("正文事件归属错误: %s", 增量.参数)
	}
	已更新 := 当前对象.事件(测试器, "conversation.updated")
	if !字节包.包含(已更新.参数, []byte(`"title_source":"generated"`)) {
		测试器.格式化报告致命错误("标题未异步更新: %s", 已更新.参数)
	}
	释放首个()
	当前对象.事件(测试器, "run.completed")
	行集 := 读取消息集(测试器, 当前对象, 会话标识)
	if len(行集) != 2 || 行集[0].标识 != 用户标识 || 行集[0].序号 != 1 || 行集[1].序号 != 2 || 行集[1].状态 != 智能体包.已完成 || 行集[1].已完成位于 == nil {
		测试器.格式化报告致命错误("消息未正确保存: %+v", 行集)
	}
	步骤集帧 := 当前对象.调用(测试器, "step.list", map[string]any{"run_id": 执行标识})
	要求成功(测试器, 步骤集帧)
	var 步骤集 []struct {
		标识   string `json:"id"`
		名称   string `json:"name"`
		状态   string `json:"status"`
		用量字节 int    `json:"usage_bytes"`
	}
	if 错误 := JSON包.反序列化(步骤集帧.结果, &步骤集); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if len(步骤集) != 2 {
		测试器.格式化报告致命错误("标题与回答没有分别记录: %s", 步骤集帧.结果)
	}
	for _, 步骤 := range 步骤集 {
		if 步骤.状态 != 智能体包.已完成 || 步骤.用量字节 == 0 {
			测试器.格式化报告致命错误("调用用量或状态丢失: %+v", 步骤)
		}
		用量 := 当前对象.调用(测试器, "step.data", map[string]any{"step_id": 步骤.标识, "field": "usage"})
		要求成功(测试器, 用量)
		if !字节包.包含(用量.结果, []byte("custom")) {
			测试器.格式化报告致命错误("usage 未原样保存: %s", 用量.结果)
		}
	}
	秒 := 当前对象.调用(测试器, "conversation.send", map[string]any{"conversation_id": 会话标识, "stream": false, "content": 消息内容块集("第二个问题")})
	秒执行 := 结果字符串(测试器, 秒, "run_id")
	秒用户 := 结果字符串(测试器, 秒, "user_message_id")
	旧回答 := 结果字符串(测试器, 秒, "message_id")
	当前对象.事件(测试器, "run.completed")
	互斥锁.加锁()
	已组装 := 历史集["第二个问题"]
	互斥锁.解锁()
	预期历史 := []string{问题, "流式正文", "第二个问题"}
	if len(已组装) < len(预期历史) || !切片包.相等(已组装[len(已组装)-len(预期历史):], 预期历史) {
		测试器.格式化报告致命错误("历史内容或顺序错误: %q", 已组装)
	}
	编辑重发 := 当前对象.调用(测试器, "conversation.resend", map[string]any{"conversation_id": 会话标识, "message_id": 秒用户, "stream": false, "content": 消息内容块集("修改后的问题")})
	新建执行 := 结果字符串(测试器, 编辑重发, "run_id")
	if 新建执行 == 秒执行 || 结果字符串(测试器, 编辑重发, "user_message_id") != 秒用户 || !字节包.包含(编辑重发.结果, []byte(旧回答)) {
		测试器.格式化报告致命错误("重发未保留用户消息并清理旧回答: %s", 编辑重发.结果)
	}
	当前对象.事件(测试器, "run.completed")
	行集 = 读取消息集(测试器, 当前对象, 会话标识)
	if len(行集) != 4 || 行集[2].序号 != 3 || 行集[2].标识 != 秒用户 || 行集[3].序号 != 4 || 行集[3].标识 == 旧回答 || 行集[3].内容[0].文本 != "回答：修改后的问题" {
		测试器.格式化报告致命错误("重发保留了旧分支: %+v", 行集)
	}
	无效 := 当前对象.调用(测试器, "conversation.resend", map[string]any{"conversation_id": 会话标识, "message_id": 用户标识, "content": 消息内容块集("不是最后一条")})
	if 无效.错误 == nil || 无效.错误.代码 != 1003 {
		测试器.格式化报告致命错误("允许编辑历史分支: %+v", 无效)
	}
	旧步骤集 := 当前对象.调用(测试器, "step.list", map[string]any{"run_id": 秒执行})
	要求成功(测试器, 旧步骤集)
	var 旧 []struct {
		标识 string `json:"id"`
	}
	if 错误 := JSON包.反序列化(旧步骤集.结果, &旧); 错误 != nil || len(旧) != 1 {
		测试器.格式化报告致命错误("旧执行记录缺失: %s", 旧步骤集.结果)
	}
	已丢弃 := 当前对象.调用(测试器, "step.data", map[string]any{"step_id": 旧[0].标识, "field": "output"})
	要求成功(测试器, 已丢弃)
	if !字节包.包含(已丢弃.结果, []byte(`"present":false`)) {
		测试器.格式化报告致命错误("旧回答副本仍保留: %s", 已丢弃.结果)
	}
	手动 := 当前对象.调用(测试器, "conversation.update", map[string]any{"id": 会话标识, "title": "人工标题"})
	要求成功(测试器, 手动)
	当前对象.停止(测试器)
	对象2 := 开始应用(测试器, 启动配置, &日志集)
	已恢复 := 对象2.调用(测试器, "conversation.get", map[string]any{"id": 会话标识})
	要求成功(测试器, 已恢复)
	if !字节包.包含(已恢复.结果, []byte("人工标题")) || len(读取消息集(测试器, 对象2, 会话标识)) != 4 {
		测试器.格式化报告致命错误("重启后历史丢失: %s", 已恢复.结果)
	}
	对象2.停止(测试器)
	if 字节包.包含([]byte(当前对象.已见.字符串()), []byte("fake-dialogue-key")) || 字节包.包含(日志集.字节(), []byte("fake-dialogue-key")) {
		测试器.报告致命错误("凭据泄露到事件或日志")
	}
}

func Test会话取消隔离并且恢复(测试器 *测试包.测试器) {
	已开始 := make(chan struct{}, 2)
	上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
		var 请求 struct {
			流   bool `json:"stream"`
			消息集 []struct {
				内容 string `json:"content"`
			} `json:"messages"`
		}
		if 错误 := JSON包.新建解码器(当前请求.正文).解码(&请求); 错误 != nil {
			测试器.错误文本(错误)
			return
		}
		if !请求.流 {
			当前响应写入器.头部().设置("Content-Type", "application/json")
			_, _ = 输入输出包.写入字符串(当前响应写入器, `{"choices":[{"index":0,"message":{"role":"assistant","content":"完成"},"finish_reason":"stop"}]}`)
			return
		}
		当前响应写入器.头部().设置("Content-Type", "text/event-stream")
		_, _ = 输入输出包.写入字符串(当前响应写入器, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"部分结果\"},\"finish_reason\":null}]}\n\n")
		当前响应写入器.(HTTP包.刷新器).刷新()
		已开始 <- struct{}{}
		<-当前请求.上下文().完成()
	}))
	测试器.注册清理函数(上游.关闭)
	启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	启动配置.最大并发执行集 = 2
	当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	消息标识 := 初始化会话模型(测试器, 当前对象, 上游.URL)
	工作区 := 当前对象.调用(测试器, "workspace.create", map[string]any{"name": "项目", "root_path": 测试器.创建临时目录()})
	工作区标识 := 结果字符串(测试器, 工作区, "id")
	已受理 := 当前对象.调用(测试器, "conversation.send", map[string]any{"workspace_id": 工作区标识, "model_id": 消息标识, "content": 消息内容块集("等待取消")})
	标识, 执行标识 := 结果字符串(测试器, 已受理, "conversation_id"), 结果字符串(测试器, 已受理, "run_id")
	当前对象.事件(测试器, "message.delta")
	<-已开始
	繁忙 := 当前对象.调用(测试器, "conversation.send", map[string]any{"conversation_id": 标识, "content": 消息内容块集("重叠发送")})
	if 繁忙.错误 == nil || 繁忙.错误.代码 != 1006 {
		测试器.格式化报告致命错误("同会话未互斥: %+v", 繁忙)
	}
	已阻止 := 当前对象.调用(测试器, "workspace.delete", map[string]any{"id": 工作区标识})
	if 已阻止.错误 == nil || 已阻止.错误.代码 != 1006 {
		测试器.格式化报告致命错误("运行中删除了工作区: %+v", 已阻止)
	}
	其他 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "stream": false, "content": 消息内容块集("另一个窗口")})
	其他执行 := 结果字符串(测试器, 其他, "run_id")
	已完成 := 当前对象.事件(测试器, "run.completed")
	if !字节包.包含(已完成.参数, []byte(其他执行)) {
		测试器.格式化报告致命错误("另一会话受阻: %s", 已完成.参数)
	}
	要求成功(测试器, 当前对象.调用(测试器, "run.cancel", map[string]any{"run_id": 执行标识}))
	当前对象.事件(测试器, "run.cancelled")
	行集 := 读取消息集(测试器, 当前对象, 标识)
	if len(行集) != 2 || 行集[1].状态 != 智能体包.已取消 || 行集[1].已完成位于 != nil || 行集[1].内容[0].文本 != "部分结果" {
		测试器.格式化报告致命错误("取消丢失部分结果: %+v", 行集)
	}
	要求成功(测试器, 当前对象.调用(测试器, "workspace.delete", map[string]any{"id": 工作区标识}))
	非活动 := 当前对象.调用(测试器, "conversation.send", map[string]any{"conversation_id": 标识, "content": 消息内容块集("已删除工作区")})
	if 非活动.错误 == nil || 非活动.错误.代码 != 1003 {
		测试器.格式化报告致命错误("在已删除工作区执行: %+v", 非活动)
	}
	要求成功(测试器, 当前对象.调用(测试器, "workspace.restore", map[string]any{"id": 工作区标识}))
	当前对象.停止(测试器)
	// 在测试独立库中构造进程退出前的活动状态，验证恢复保留已保存内容。
	数据库, 错误 := SQLite包.打开(测试器.上下文(), 启动配置.数据库路径)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 数据库.数据库.进程执行("UPDATE conversation_runs SET status='running',finished_at=NULL WHERE id=?", 执行标识).错误; 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 数据库.数据库.进程执行("UPDATE conversation_messages SET status='running' WHERE run_id=? AND role='assistant'", 执行标识).错误; 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 数据库.关闭(); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	对象2 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	已恢复 := 对象2.调用(测试器, "run.get", map[string]any{"id": 执行标识})
	要求成功(测试器, 已恢复)
	if !字节包.包含(已恢复.结果, []byte(`"status":"interrupted"`)) {
		测试器.格式化报告致命错误("未确认中断状态: %s", 已恢复.结果)
	}
	行集 = 读取消息集(测试器, 对象2, 标识)
	if 行集[1].状态 != 智能体包.已中断 || 行集[1].内容[0].文本 != "部分结果" {
		测试器.格式化报告致命错误("恢复覆盖了部分结果: %+v", 行集)
	}
	对象2.停止(测试器)
}
```
