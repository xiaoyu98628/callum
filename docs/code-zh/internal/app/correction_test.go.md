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
	配置模块包 "github.com/xiaoyu98628/callum/internal/config"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	词元包 "github.com/xiaoyu98628/callum/internal/model/tokens"
	设置包 "github.com/xiaoyu98628/callum/internal/settings"
)

func 纠正调用(标识, 名称, 参数 string, 流 bool, 索引 int) map[string]any {
	调用 := map[string]any{"id": 标识, "type": "function", "function": map[string]string{"name": 名称, "arguments": 参数}}
	if 流 {
		调用["index"] = 索引
	}
	return 调用
}
func 纠正JSON(测试器 *测试包.测试器, 值 any) string {
	测试器.标记测试辅助函数()
	数据, 错误 := JSON包.序列化(值)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	return string(数据)
}
func 纠正步骤集(测试器 *测试包.测试器, 当前应用通信对端 *应用通信对端, 执行标识 string) []智能体包.步骤摘要 {
	测试器.标记测试辅助函数()
	帧 := 当前应用通信对端.调用(测试器, "step.list", map[string]any{"run_id": 执行标识, "limit": 200})
	要求成功(测试器, 帧)
	var 步骤集 []智能体包.步骤摘要
	if 错误 := JSON包.反序列化(帧.结果, &步骤集); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	return 步骤集
}
func 纠正数据(测试器 *测试包.测试器, 当前应用通信对端 *应用通信对端, 步骤标识, 字段 string) string {
	测试器.标记测试辅助函数()
	帧 := 当前应用通信对端.调用(测试器, "step.data", map[string]any{"step_id": 步骤标识, "field": 字段})
	要求成功(测试器, 帧)
	var 页 struct {
		片段 string
		完成 bool
	}
	if 错误 := JSON包.反序列化(帧.结果, &页); 错误 != nil || !页.完成 {
		测试器.格式化报告致命错误("步骤审计读取失败: %s %v", 帧.结果, 错误)
	}
	return 页.片段
}

func Test格式错误参数纠正保留审计并且审批(测试器 *测试包.测试器) {
	测试器.并行()
	for _, 流 := range []bool{false, true} {
		测试器.执行(格式化包.转为字符串(流), func(测试器 *测试包.测试器) {
			根, 外部 := 测试器.创建临时目录(), 测试器.创建临时目录()
			标记, 从不 := 文件路径包.合并(外部, "corrected.txt"), 文件路径包.合并(外部, "never.txt")
			if 错误 := 操作系统包.写入文件(文件路径包.合并(根, "read.txt"), []byte("already-read"), 0600); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			var main调用集 原子操作包.整数32
			服务端 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
				var 输入 struct {
					流   bool
					工具集 []JSON包.原始JSON消息
					消息集 []struct {
						角色, 内容 string
						调用集    []struct{ 标识 string } `json:"tool_calls"`
					}
				}
				if 错误 := JSON包.新建解码器(当前请求.正文).解码(&输入); 错误 != nil {
					测试器.错误文本(错误)
					当前响应写入器.写入头部(400)
					return
				}
				if len(输入.工具集) == 0 {
					写入文件工具响应(测试器, 当前响应写入器, 输入.流, map[string]any{"content": "title"}, "stop")
					return
				}
				数量 := main调用集.添加(1)
				读取数量, 无效数量, 反馈 := 0, 0, false
				for _, 映射 := range 输入.消息集 {
					if 字符串包.包含(映射.内容, "服务端工具参数校验反馈") {
						反馈 = true
					}
					for _, 调用 := range 映射.调用集 {
						if 调用.标识 == "read" {
							读取数量++
						}
						if 字符串包.存在前缀(调用.标识, "bad-") {
							无效数量++
						}
					}
				}
				if 数量 >= 3 && (读取数量 != 1 || 无效数量 != 0) {
					测试器.格式化创建错误("纠正重放了先前操作或将坏批次带入上下文: read=%d bad=%d", 读取数量, 无效数量)
				}
				if 数量 == 3 && !反馈 {
					测试器.错误文本("未发送服务端纠正信息")
				}
				if 数量 > 3 && 反馈 {
					测试器.错误文本("纠正信息污染了后续历史")
				}
				switch 数量 {
				case 1:
					参数 := 纠正JSON(测试器, map[string]any{"path": "read.txt"})
					写入文件工具响应(测试器, 当前响应写入器, 输入.流, map[string]any{"tool_calls": []any{纠正调用("read", "read_file", 参数, 输入.流, 0)}}, "tool_calls")
				case 2:
					补丁 := 纠正JSON(测试器, map[string]any{"patch": "*** Begin Patch\n*** Add File: " + 文件路径包.目标斜杠(从不) + "\n+must not execute\n*** End Patch\n", "base_versions": []any{}})
					写入文件工具响应(测试器, 当前响应写入器, 输入.流, map[string]any{"content": "bad batch", "tool_calls": []any{纠正调用("bad-valid", "apply_patch", 补丁, 输入.流, 0), 纠正调用("bad-json", "read_file", `{"["string""]</｜｜DSML｜｜: `, 输入.流, 1)}}, "tool_calls")
				case 3:
					补丁 := 纠正JSON(测试器, map[string]any{"patch": "*** Begin Patch\n*** Add File: " + 文件路径包.目标斜杠(标记) + "\n+corrected only\n*** End Patch\n", "base_versions": []any{}})
					写入文件工具响应(测试器, 当前响应写入器, 输入.流, map[string]any{"tool_calls": []any{纠正调用("corrected", "apply_patch", 补丁, 输入.流, 0)}}, "tool_calls")
				default:
					写入文件工具响应(测试器, 当前响应写入器, 输入.流, map[string]any{"content": "done"}, "stop")
				}
			}))
			测试器.注册清理函数(服务端.关闭)
			启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
			消息标识 := 初始化会话模型(测试器, 当前对象, 服务端.URL)
			工作区标识 := 结果字符串(测试器, 当前对象.调用(测试器, "workspace.create", map[string]any{"name": "project", "root_path": 根}), "id")
			已受理 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "workspace_id": 工作区标识, "stream": 流, "content": 消息内容块集("recover")})
			要求成功(测试器, 已受理)
			执行标识, 会话标识 := 结果字符串(测试器, 已受理, "run_id"), 结果字符串(测试器, 已受理, "conversation_id")
			事件 := 当前对象.事件(测试器, "approval.requested")
			var 待处理 struct {
				审批标识 string  `json:"approval_id"`
				审批   智能体包.审批 `json:"approval"`
			}
			if 错误 := JSON包.反序列化(事件.参数, &待处理); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			if main调用集.加载() != 3 || 待处理.审批.请求哈希 == "" || !字节包.包含(待处理.审批.请求, []byte("corrected.txt")) {
				测试器.格式化报告致命错误("纠正后未重新生成审批: calls=%d %s", main调用集.加载(), 事件.参数)
			}
			for _, 文件 := range []string{从不, 标记} {
				if _, 错误 := 操作系统包.状态信息(文件); !操作系统包.是否不存在(错误) {
					测试器.格式化报告致命错误("审批前或坏批次已经执行: %s %v", 文件, 错误)
				}
			}
			要求成功(测试器, 当前对象.调用(测试器, "approval.decide", map[string]any{"approval_id": 待处理.审批标识, "decision": "approved"}))
			当前对象.事件(测试器, "run.completed")
			数据, 错误 := 操作系统包.读取文件(标记)
			if 错误 != nil || string(数据) != "corrected only\n" {
				测试器.格式化报告致命错误("纠正后的操作没有完成: %q %v", 数据, 错误)
			}
			if _, 错误 := 操作系统包.状态信息(从不); !操作系统包.是否不存在(错误) {
				测试器.报告致命错误("执行了坏批次中的有效调用")
			}
			工具数量, 失败模型, 纠正集 := 0, 0, 0
			for _, 步骤 := range 纠正步骤集(测试器, 当前对象, 执行标识) {
				if 步骤.类型 == "tool_call" {
					工具数量++
				}
				if 步骤.类型 == "model_call" {
					输入 := 纠正数据(测试器, 当前对象, 步骤.标识, "input")
					if 字符串包.包含(输入, "tool_arguments_correction") {
						纠正集++
					}
					if 步骤.状态 == 智能体包.失败 {
						失败模型++
						输出 := 纠正数据(测试器, 当前对象, 步骤.标识, "output")
						if !字符串包.包含(输出, "DSML") || 字符串包.包含(输出, "context_message") || 步骤.用量字节 == 0 {
							测试器.格式化报告致命错误("坏响应审计或 usage 丢失: %s", 输出)
						}
					}
				}
			}
			if 工具数量 != 2 || 失败模型 != 1 || 纠正集 != 1 {
				测试器.格式化报告致命错误("步骤未保持操作次数和纠正来源: tools=%d failed=%d corrections=%d", 工具数量, 失败模型, 纠正集)
			}
			// 后续会话恢复只保留已执行事实，不恢复坏批次或临时纠正指令。
			要求成功(测试器, 当前对象.调用(测试器, "conversation.send", map[string]any{"conversation_id": 会话标识, "model_id": 消息标识, "stream": 流, "content": 消息内容块集("继续")}))
			当前对象.事件(测试器, "run.completed")
		})
	}
}

func Test重复格式错误参数停止无执行中(测试器 *测试包.测试器) {
	测试器.并行()
	for _, 模式 := range []string{"json", "schema", "limit"} {
		测试器.执行(模式, func(测试器 *测试包.测试器) {
			var 数量 原子操作包.整数32
			服务端 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
				var 输入 struct {
					流   bool
					工具集 []JSON包.原始JSON消息
					消息集 []struct{ 角色, 内容 string }
				}
				if JSON包.新建解码器(当前请求.正文).解码(&输入) != nil {
					当前响应写入器.写入头部(400)
					return
				}
				if len(输入.工具集) == 0 {
					写入文件工具响应(测试器, 当前响应写入器, 输入.流, map[string]any{"content": "title"}, "stop")
					return
				}
				数量 := 数量.添加(1)
				参数 := `{"DSML`
				if 模式 == "schema" {
					参数 = `{"command":"[\"git\",\"status\"]"}`
				}
				if 模式 == "limit" {
					参数 = `{"command":"` + 字符串包.重复("x", 17000) + `"}`
				}
				if 数量 == 2 && 模式 == "schema" && !字符串包.包含(输入.消息集[len(输入.消息集)-1].内容, "字符串数组") {
					测试器.错误文本("数组类型错误没有字段级反馈")
				}
				写入文件工具响应(测试器, 当前响应写入器, 输入.流, map[string]any{"tool_calls": []any{纠正调用(格式化包.转为字符串(数量), "run_command", 参数, 输入.流, 0)}}, "tool_calls")
			}))
			测试器.注册清理函数(服务端.关闭)
			启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
			消息标识 := 初始化会话模型(测试器, 当前对象, 服务端.URL)
			已受理 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "stream": true, "content": 消息内容块集("repeat")})
			要求成功(测试器, 已受理)
			终态 := 当前对象.事件(测试器, "run.failed")
			if !字节包.包含(终态.参数, []byte("没有执行进展")) || 数量.加载() != 2 {
				测试器.格式化报告致命错误("重复坏参数没有明确终止: count=%d %s", 数量.加载(), 终态.参数)
			}
			执行标识 := 结果字符串(测试器, 已受理, "run_id")
			for _, 步骤 := range 纠正步骤集(测试器, 当前对象, 执行标识) {
				if 步骤.类型 == "tool_call" && 步骤.状态 != 智能体包.失败 {
					测试器.格式化报告致命错误("执行了坏参数工具: %+v", 步骤)
				}
			}
		})
	}
}

func Test参数纠正取消并且会话隔离(测试器 *测试包.测试器) {
	测试器.并行()
	等待中, 已取消 := make(chan struct{}), make(chan struct{})
	var 无效调用集 原子操作包.整数32
	服务端 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
		var 输入 struct {
			流   bool
			工具集 []JSON包.原始JSON消息
			消息集 []struct{ 角色, 内容 string }
		}
		if JSON包.新建解码器(当前请求.正文).解码(&输入) != nil {
			当前响应写入器.写入头部(400)
			return
		}
		if len(输入.工具集) == 0 {
			写入文件工具响应(测试器, 当前响应写入器, 输入.流, map[string]any{"content": "title"}, "stop")
			return
		}
		无效, 反馈 := false, false
		for _, 映射 := range 输入.消息集 {
			if 映射.角色 == "user" && 映射.内容 == "bad-isolated" {
				无效 = true
			}
			if 字符串包.包含(映射.内容, "服务端工具参数校验反馈") {
				反馈 = true
			}
		}
		if !无效 {
			if 反馈 {
				测试器.错误文本("参数纠正信息串入其他会话")
			}
			写入文件工具响应(测试器, 当前响应写入器, 输入.流, map[string]any{"content": "healthy"}, "stop")
			return
		}
		数量 := 无效调用集.添加(1)
		if 数量 == 1 {
			写入文件工具响应(测试器, 当前响应写入器, 输入.流, map[string]any{"tool_calls": []any{纠正调用("bad", "read_file", "{", 输入.流, 0)}}, "tool_calls")
			return
		}
		if 数量 != 2 || !反馈 {
			测试器.错误文本("纠正请求状态错误")
			当前响应写入器.写入头部(500)
			return
		}
		close(等待中)
		<-当前请求.上下文().完成()
		close(已取消)
	}))
	测试器.注册清理函数(服务端.关闭)
	启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	启动配置.最大并发执行集 = 2
	当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	消息标识 := 初始化会话模型(测试器, 当前对象, 服务端.URL)
	无效 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "stream": true, "content": 消息内容块集("bad-isolated")})
	要求成功(测试器, 无效)
	执行上下文, 取消函数 := 上下文包.创建超时上下文(测试器.上下文(), 3*时间包.秒)
	defer 取消函数()
	select {
	case <-等待中:
	case <-执行上下文.完成():
		测试器.报告致命错误("没有进入纠正请求:", 执行上下文.获取错误())
	}
	正常 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "stream": false, "content": 消息内容块集("healthy-isolated")})
	要求成功(测试器, 正常)
	当前对象.事件(测试器, "run.completed")
	for _, 步骤 := range 纠正步骤集(测试器, 当前对象, 结果字符串(测试器, 正常, "run_id")) {
		if 步骤.类型 == "model_call" && 字符串包.包含(纠正数据(测试器, 当前对象, 步骤.标识, "input"), "tool_arguments_correction") {
			测试器.报告致命错误("纠正审计串入其他执行")
		}
	}
	要求成功(测试器, 当前对象.调用(测试器, "run.cancel", map[string]any{"run_id": 结果字符串(测试器, 无效, "run_id")}))
	当前对象.事件(测试器, "run.cancelled")
	select {
	case <-已取消:
	case <-执行上下文.完成():
		测试器.报告致命错误("取消未传递给纠正请求:", 执行上下文.获取错误())
	}
	if 无效调用集.加载() != 2 {
		测试器.报告致命错误("取消后仍启动纠正")
	}
}

func Test参数纠正反馈计数朝向预算(测试器 *测试包.测试器) {
	测试器.并行()
	var 数量 原子操作包.整数32
	var 基线 原子操作包.整数64
	服务端 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
		var 输入 struct {
			流   bool
			工具集 []struct{ 函数 模型包.工具定义 }
			消息集 []模型包.消息
		}
		if JSON包.新建解码器(当前请求.正文).解码(&输入) != nil {
			当前响应写入器.写入头部(400)
			return
		}
		if len(输入.工具集) == 0 {
			写入文件工具响应(测试器, 当前响应写入器, 输入.流, map[string]any{"content": "title"}, "stop")
			return
		}
		数量 := 数量.添加(1)
		if 数量 == 1 {
			var 定义集 []模型包.工具定义
			for _, 项 := range 输入.工具集 {
				定义集 = append(定义集, 项.函数)
			}
			基线.存储(词元包.估算(输入.消息集, 定义集, 0))
			写入文件工具响应(测试器, 当前响应写入器, 输入.流, map[string]any{"content": "baseline"}, "stop")
			return
		}
		写入文件工具响应(测试器, 当前响应写入器, 输入.流, map[string]any{"tool_calls": []any{纠正调用("bad", "read_file", "{", 输入.流, 0)}}, "tool_calls")
	}))
	测试器.注册清理函数(服务端.关闭)
	启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	消息标识 := 初始化会话模型(测试器, 当前对象, 服务端.URL)
	要求成功(测试器, 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "stream": false, "content": 消息内容块集("same question")}))
	当前对象.事件(测试器, "run.completed")
	压缩 := 设置包.默认压缩()
	压缩.已启用 = true
	压缩.词元预算 = 基线.加载() + 1
	压缩设置(测试器, 当前对象, 压缩)
	要求成功(测试器, 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "stream": false, "content": 消息内容块集("same question")}))
	终态 := 当前对象.事件(测试器, "run.failed")
	if 数量.加载() != 2 || !字节包.包含(终态.参数, []byte("token_budget_exceeded")) {
		测试器.格式化报告致命错误("纠正信息绕过预算或第一轮未进入模型: calls=%d %s", 数量.加载(), 终态.参数)
	}
}
```
