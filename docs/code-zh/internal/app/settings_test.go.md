```go
package 应用模块

import (
	JSON包 "encoding/json"
	错误判断包 "errors"
	输入输出包 "io"
	结构化日志包 "log/slog"
	HTTP包 "net/http"
	HTTP测试包 "net/http/httptest"
	字符串转换包 "strconv"
	字符串包 "strings"
	测试包 "testing"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	事件集包 "github.com/xiaoyu98628/callum/internal/agent/events"
	设置包 "github.com/xiaoyu98628/callum/internal/settings"
	SQLite包 "github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

func 读取设置(测试器 *测试包.测试器, 当前应用通信对端 *应用通信对端) 设置包.记录 {
	测试器.标记测试辅助函数()
	帧 := 当前应用通信对端.调用(测试器, "settings.get", map[string]any{"key": "execution"})
	要求成功(测试器, 帧)
	var 值 设置包.记录
	if 错误 := JSON包.反序列化(帧.结果, &值); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	return 值
}

func Test无效已存储设置释放启动资源集(测试器 *测试包.测试器) {
	运行时, 启动配置 := 运行时测试夹具(测试器, 1)
	if 错误 := 运行时.数据库.数据库.表("app_settings").条件("key=?", 设置包.执行键).更新("value", `{"approval_policy":"never","approvals_reviewer":"auto_review"}`).错误; 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 运行时.关闭(); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 失败, 错误 := 新建测试运行时(测试器.上下文(), 启动配置, 结构化日志包.新建(结构化日志包.新建JSON处理器(输入输出包.丢弃, nil))); 错误 == nil {
		失败.关闭()
		测试器.报告致命错误("非法持久化配置被默认权限替代")
	} else if !错误判断包.是否属于(错误, 设置包.存储错误) {
		测试器.格式化报告致命错误("启动错误丢失设置来源: %v", 错误)
	}
	// 只修复临时测试库，之后应能重新取得实例锁并正常启动。
	数据库, 错误 := SQLite包.打开(测试器.上下文(), 启动配置.数据库路径)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	错误 = 数据库.数据库.表("app_settings").条件("key=?", 设置包.执行键).更新("value", `{"approval_policy":"on-request","approvals_reviewer":"user"}`).错误
	关闭错误 := 数据库.关闭()
	if 错误 != nil || 关闭错误 != nil {
		测试器.格式化报告致命错误("临时设置修复失败: %v %v", 错误, 关闭错误)
	}
	已重启, 错误 := 新建测试运行时(测试器.上下文(), 启动配置, 结构化日志包.新建(结构化日志包.新建JSON处理器(输入输出包.丢弃, nil)))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 已重启.关闭(); 错误 != nil {
		测试器.报告致命错误(错误)
	}
}

func Test设置标准输入输出冲突并且重启(测试器 *测试包.测试器) {
	运行时, 启动配置 := 运行时测试夹具(测试器, 1)
	首个, 秒 := 共享通信对端(测试器, 运行时), 共享通信对端(测试器, 运行时)
	初始 := 读取设置(测试器, 首个)
	设置审批策略(测试器, 首个, "always")
	已保存 := 读取设置(测试器, 秒)
	if 已保存.修订号 != 初始.修订号+1 || 已保存.标识 != 初始.标识 {
		测试器.格式化报告致命错误("其他连接未读到新设置: %+v", 已保存)
	}
	冲突 := 秒.调用(测试器, "settings.update", map[string]any{"key": "execution", "revision": 初始.修订号,
		"value": map[string]any{"approval_policy": "never", "approvals_reviewer": "user"}})
	if 冲突.错误 == nil || 冲突.错误.代码 != 1003 {
		测试器.报告致命错误("过期设置覆盖了新设置")
	}
	for _, 参数 := range []map[string]any{
		{"key": "unknown", "revision": 已保存.修订号, "value": map[string]any{}},
		{"key": "execution", "revision": 0, "value": map[string]any{}},
		{"key": "execution", "revision": 已保存.修订号, "value": nil},
		{"key": "execution", "revision": 已保存.修订号, "value": map[string]any{"approval_policy": "never", "approvals_reviewer": "auto_review"}},
	} {
		帧 := 秒.调用(测试器, "settings.update", 参数)
		if 帧.错误 == nil || 帧.错误.代码 != -32602 {
			测试器.格式化报告致命错误("非法配置未拒绝: %+v", 帧)
		}
	}
	首个.停止(测试器)
	秒.停止(测试器)
	if 错误 := 运行时.关闭(); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	已重启, 错误 := 新建测试运行时(测试器.上下文(), 启动配置, 结构化日志包.新建(结构化日志包.新建JSON处理器(输入输出包.丢弃, nil)))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	defer func() {
		if 错误 := 已重启.关闭(); 错误 != nil {
			测试器.错误文本(错误)
		}
	}()
	值, 错误 := 已重启.获取设置().获取(测试器.上下文(), 设置包.执行键)
	if 错误 != nil || 值.修订号 != 已保存.修订号 || string(值.值) != string(已保存.值) || 值.标识 != 已保存.标识 {
		测试器.格式化报告致命错误("重启覆盖了用户配置: %+v %v", 值, 错误)
	}
}

func Test设置更新隔离运行中审批并且新建执行(测试器 *测试包.测试器) {
	type 投影 struct {
		来源  string `json:"source"`
		修订号 int64  `json:"settings_revision"`
		策略  string `json:"approval_policy"`
		问题  string `json:"-"`
	}
	投影集 := make(chan 投影, 8)
	上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
		var 输入 struct {
			工具集 []JSON包.原始JSON消息 `json:"tools"`
			消息集 []struct {
				角色 string `json:"role"`
				内容 string `json:"content"`
			} `json:"messages"`
		}
		if 错误 := JSON包.新建解码器(当前请求.正文).解码(&输入); 错误 != nil {
			测试器.错误文本(错误)
			当前响应写入器.写入头部(400)
			return
		}
		当前响应写入器.头部().设置("Content-Type", "application/json")
		if len(输入.工具集) > 0 {
			var 值 投影
			系统 := 输入.消息集[0].内容
			if JSON包.反序列化([]byte(系统[字符串包.最后索引(系统, "\n")+1:]), &值) != nil {
				测试器.错误文本("主请求缺少执行设置投影")
			}
			for _, 消息 := range 输入.消息集 {
				if 消息.角色 == "user" && (消息.内容 == "旧任务" || 消息.内容 == "新任务") {
					值.问题 = 消息.内容
				}
			}
			select {
			case 投影集 <- 值:
			default:
				测试器.错误文本("模型请求超出预期轮次")
				当前响应写入器.写入头部(HTTP包.状态内部服务端错误)
				return
			}
		}
		数量 := 0
		for _, 映射 := range 输入.消息集 {
			if 映射.角色 == "tool" {
				数量++
			}
		}
		if len(输入.工具集) == 0 || 数量 >= 2 {
			_, _ = 输入输出包.写入字符串(当前响应写入器, `{"choices":[{"index":0,"message":{"role":"assistant","content":"完成"},"finish_reason":"stop"}]}`)
			return
		}
		_, _ = 输入输出包.写入字符串(当前响应写入器, `{"choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"call-`+字符串转换包.整数转字符串(数量+1)+`","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"a.txt\"}"}}]},"finish_reason":"tool_calls"}]}`)
	}))
	测试器.注册清理函数(上游.关闭)
	运行时, _ := 运行时测试夹具(测试器, 2)
	当前对象 := 共享通信对端(测试器, 运行时)
	订阅, 错误 := 运行时.订阅(事件集包.筛选{类型集: []string{"approval.requested", "run.completed", "run.failed"}})
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	defer 订阅.关闭()
	消息标识, 工作区标识 := 初始化会话模型(测试器, 当前对象, 上游.URL), 初始化工具工作区(测试器, 当前对象)
	设置审批策略(测试器, 当前对象, "always")
	旧设置 := 读取设置(测试器, 当前对象)
	内部发送 := func(文本 string) string {
		return 结果字符串(测试器, 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "workspace_id": 工作区标识, "stream": false, "content": 消息内容块集(文本)}), "run_id")
	}
	旧执行 := 内部发送("旧任务")
	首个审批 := 下一个业务(测试器, 订阅, "approval.requested", 旧执行).数据.(智能体包.审批)
	设置审批策略(测试器, 当前对象, "on-request")
	新建设置 := 读取设置(测试器, 当前对象)
	新建执行 := 内部发送("新任务")
	下一个业务(测试器, 订阅, "run.completed", 新建执行)
	新建状态, 错误 := 运行时.执行集().获取(测试器.上下文(), 新建执行)
	if 错误 != nil || 新建状态.设置快照 == nil || 新建状态.设置快照.修订号 != 新建设置.修订号 || 新建状态.设置快照.审批策略 != "on-request" {
		测试器.格式化报告致命错误("新执行没有读取新设置: %+v %v", 新建状态, 错误)
	}
	行集, 错误 := 运行时.获取审批集().列表(测试器.上下文(), 智能体包.审批筛选{执行标识: 新建执行})
	if 错误 != nil || len(行集) != 0 {
		测试器.格式化报告致命错误("新执行仍使用旧策略: %+v %v", 行集, 错误)
	}
	旧状态, 错误 := 运行时.执行集().获取(测试器.上下文(), 旧执行)
	if 错误 != nil || 旧状态.状态 != 智能体包.等待中审批 || 旧状态.设置快照 == nil || 旧状态.设置快照.修订号 != 旧设置.修订号 {
		测试器.格式化报告致命错误("更新干扰了待审批任务: %+v %v", 旧状态, 错误)
	}
	要求成功(测试器, 当前对象.调用(测试器, "approval.decide", map[string]any{"approval_id": 首个审批.标识, "decision": "approved"}))
	秒审批 := 下一个业务(测试器, 订阅, "approval.requested", 旧执行).数据.(智能体包.审批)
	if 秒审批.标识 == 首个审批.标识 {
		测试器.报告致命错误("同一次审批被当作下一步授权")
	}
	要求成功(测试器, 当前对象.调用(测试器, "approval.decide", map[string]any{"approval_id": 秒审批.标识, "decision": "approved"}))
	下一个业务(测试器, 订阅, "run.completed", 旧执行)
	for range 6 { // 两个任务各经过两轮工具调用及一轮最终回答。
		var 值 投影
		select {
		case 值 = <-投影集:
		case <-测试器.上下文().完成():
			测试器.报告致命错误(测试器.上下文().获取错误())
		}
		预期 := 旧设置
		if 值.问题 == "新任务" {
			预期 = 新建设置
		} else if 值.问题 != "旧任务" {
			测试器.格式化报告致命错误("无法确定请求所属任务: %+v", 值)
		}
		var 执行 设置包.执行
		if 错误 := JSON包.反序列化(预期.值, &执行); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		if 值.来源 != "server_execution_settings" || 值.修订号 != 预期.修订号 || 值.策略 != string(执行.审批策略) {
			测试器.格式化报告致命错误("模型请求混用了其他执行的设置: %+v want=%+v", 值, 预期)
		}
	}
	步骤集, 错误 := 运行时.执行集().步骤集(测试器.上下文(), 旧执行, 0, 20)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	for _, 步骤 := range 步骤集 {
		if 步骤.类型 != "tool_call" {
			continue
		}
		var 输入 struct {
			修订号 int64  `json:"settings_revision"`
			策略  string `json:"approval_policy"`
		}
		if 错误 := JSON包.反序列化(步骤.输入, &输入); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		if 输入.修订号 != 旧设置.修订号 || 输入.策略 != "always" {
			测试器.格式化报告致命错误("运行中任务混用了新设置: %s", 步骤.输入)
		}
	}
	当前对象.停止(测试器)
}
```
