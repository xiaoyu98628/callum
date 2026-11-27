```go
package 应用模块

import (
	基准64包 "encoding/base64"
	JSON包 "encoding/json"
	错误判断包 "errors"
	输入输出包 "io"
	HTTP包 "net/http"
	HTTP测试包 "net/http/httptest"
	操作系统包 "os"
	进程执行包 "os/exec"
	文件路径包 "path/filepath"
	运行时包 "runtime"
	字符串包 "strings"
	原子操作包 "sync/atomic"
	测试包 "testing"
	时间包 "time"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	配置模块包 "github.com/xiaoyu98628/callum/internal/config"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	执行器包 "github.com/xiaoyu98628/callum/internal/tool/executor"
)

func 命令辅助(测试器 *测试包.测试器) string {
	测试器.标记测试辅助函数()
	名称 := "command helper"
	if 运行时包.目标操作系统 == "windows" {
		名称 += ".exe"
	}
	路径 := 文件路径包.合并(测试器.创建临时目录(), 名称)
	命令 := 进程执行包.命令上下文(测试器.上下文(), 文件路径包.合并(运行时包.Go安装根目录(), "bin", "go"), "build", "-mod=readonly", "-buildvcs=false", "-o", 路径, "../tool/builtin/command/run/testdata/process")
	if 输出, 错误 := 命令.已合并输出(); 错误 != nil {
		测试器.格式化报告致命错误("构建命令测试进程: %s %v", 输出, 错误)
	}
	return 路径
}

func 命令步骤输出(测试器 *测试包.测试器, 当前应用通信对端 *应用通信对端, 执行标识 string) (智能体包.步骤摘要, JSON包.原始JSON消息) {
	测试器.标记测试辅助函数()
	帧 := 当前应用通信对端.调用(测试器, "step.list", map[string]any{"run_id": 执行标识})
	要求成功(测试器, 帧)
	var 行集 []智能体包.步骤摘要
	if 错误 := JSON包.反序列化(帧.结果, &行集); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	for _, 行 := range 行集 {
		if 行.名称 == "run_command" {
			var 原始 字符串包.构建器
			for 偏移 := 0; ; {
				帧 := 当前应用通信对端.调用(测试器, "step.data", map[string]any{"step_id": 行.标识, "field": "output", "offset": 偏移})
				要求成功(测试器, 帧)
				var 部分 struct {
					片段  string
					完成  bool
					下一个 int `json:"next_offset"`
				}
				if 错误 := JSON包.反序列化(帧.结果, &部分); 错误 != nil {
					测试器.报告致命错误(错误)
				}
				原始.写入字符串(部分.片段)
				if 部分.完成 {
					return 行, JSON包.原始JSON消息(原始.字符串())
				}
				偏移 = 部分.下一个
			}
		}
	}
	测试器.报告致命错误("缺少命令步骤")
	return 智能体包.步骤摘要{}, nil
}

func Test命令工具集通过标准输入输出(测试器 *测试包.测试器) {
	// 各用例独占临时目录、运行时及受控端点，可与其他应用回归并行。
	测试器.并行()
	辅助 := 命令辅助(测试器)
	for _, 测试 := range []struct {
		名称, 策略, 决定, 模式 string
		边界, 流          bool
	}{
		{"sandbox_automatic", "on-request", "", "read", true, false},
		{"sandbox_never_automatic", "never", "", "read", true, false},
		{"sandbox_unlink", "on-request", "", "unlink", true, false},
		{"sandbox_script_unlink", "on-request", "", "script_unlink", true, false},
		{"sandbox_workspace_approve", "on-request", "approved", "write", true, false},
		{"sandbox_workspace_never", "never", "", "write", true, false},
		{"workspace_approve", "on-request", "approved", "write", true, false},
		{"absolute_cwd_stream", "on-request", "approved", "stream", false, true},
		{"always_reject", "always", "rejected", "write", true, true},
		{"never_denies", "never", "", "write", true, false},
		{"cancel_approval", "on-request", "cancel", "write", true, false},
		{"nonzero_exit", "on-request", "approved", "exit", true, false},
		{"cancel_running", "on-request", "approved", "cancel", true, true},
		{"tool_timeout", "on-request", "approved", "timeout", true, false},
		{"shutdown_children", "on-request", "approved", "shutdown", true, false},
	} {
		测试器.执行(测试.名称, func(测试器 *测试包.测试器) {
			沙箱内 := 字符串包.存在前缀(测试.名称, "sandbox_")
			删除 := 测试.模式 == "unlink" || 测试.模式 == "script_unlink"
			自动 := 测试.模式 == "read" || 删除
			if 沙箱内 && 运行时包.目标操作系统 != "darwin" {
				测试器.跳过("该平台尚未接入自动命令沙箱")
			}
			工作目录 := 测试器.创建临时目录()
			标记 := 文件路径包.合并(工作目录, "marker")
			释放 := 文件路径包.合并(工作目录, "release")
			心跳 := 文件路径包.合并(工作目录, "heartbeat")
			命令 := []string{辅助, "write", 标记}
			if 删除 {
				if 错误 := 操作系统包.写入文件(标记, []byte("preserved"), 0600); 错误 != nil {
					测试器.报告致命错误(错误)
				}
			}
			switch 测试.模式 {
			case "unlink":
				命令 = []string{"/bin/unlink", 标记}
			case "script_unlink":
				命令 = []string{"/bin/sh", "-c", `/bin/unlink "$1"`, "sh", 标记}
			case "read":
				命令 = []string{"/bin/pwd"}
			case "stream":
				命令 = []string{辅助, "stream", 释放}
			case "exit":
				命令 = []string{辅助, "exit", "7"}
			case "cancel", "timeout":
				命令 = []string{辅助, "sleep"}
			case "shutdown":
				命令 = []string{辅助, "spawn", 心跳}
			}
			// 本组覆盖明确的沙箱外逐次审批；普通沙箱命令在独立用例验证自动执行。
			参数 := map[string]any{"command": 命令, "sandbox_permissions": "require_escalated", "justification": "仅在受控临时目录验证本次审批和进程生命周期"}
			if 自动 {
				delete(参数, "sandbox_permissions")
				delete(参数, "justification")
			} else if 沙箱内 {
				参数["sandbox_permissions"] = "workspace_write"
			}
			if !测试.边界 {
				参数["cwd"] = 工作目录
			}
			原始参数, 错误 := JSON包.序列化(参数)
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			var 结果集 原子操作包.整数32
			上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
				var 输入 struct {
					流   bool
					工具集 []JSON包.原始JSON消息
					消息集 []struct {
						角色, 内容 string
						工具调用标识 string `json:"tool_call_id"`
					}
				}
				if 错误 := JSON包.新建解码器(当前请求.正文).解码(&输入); 错误 != nil || len(输入.消息集) == 0 {
					测试器.格式化创建错误("模型请求无效: %v", 错误)
					当前响应写入器.写入头部(400)
					return
				}
				if len(输入.工具集) == 0 {
					写入文件工具响应(测试器, 当前响应写入器, 输入.流, map[string]any{"content": "命令测试"}, "stop")
					return
				}
				if len(输入.工具集) != 9 {
					测试器.格式化创建错误("工具数量不符: %d", len(输入.工具集))
				}
				最后 := 输入.消息集[len(输入.消息集)-1]
				if 最后.角色 == "tool" {
					结果集.添加(1)
					var 输出 struct {
						操作 工具包.操作结果记录 `json:"operation"`
					}
					if 错误 := JSON包.反序列化([]byte(最后.内容), &输出); 错误 != nil {
						测试器.错误文本(错误)
					}
					预期 := 工具包.操作结果已生效
					if 测试.决定 == "rejected" || 测试.策略 == "never" && !自动 {
						预期 = 工具包.操作结果不已生效
					}
					if 测试.模式 == "timeout" {
						预期 = 工具包.操作结果未知
					}
					if 输出.操作.操作结果 != 预期 {
						测试器.格式化创建错误("模型收到错误操作事实: %s", 最后.内容)
					}
					写入文件工具响应(测试器, 当前响应写入器, 输入.流, map[string]any{"content": "已取得命令结果"}, "stop")
					return
				}
				调用 := map[string]any{"id": "command-call", "type": "function", "function": map[string]any{"name": "run_command", "arguments": string(原始参数)}}
				if 输入.流 {
					调用["index"] = 0
				}
				写入文件工具响应(测试器, 当前响应写入器, 输入.流, map[string]any{"tool_calls": []any{调用}}, "tool_calls")
			}))
			测试器.注册清理函数(上游.关闭)
			启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			if 测试.模式 == "timeout" {
				启动配置.工具超时 = 200 * 时间包.毫秒
			}
			当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
			设置审批策略(测试器, 当前对象, 测试.策略)
			消息标识 := 初始化会话模型(测试器, 当前对象, 上游.URL)
			参数 := map[string]any{"model_id": 消息标识, "stream": 测试.流, "content": 消息内容块集("执行受控命令")}
			if 测试.边界 {
				参数["workspace_id"] = 结果字符串(测试器, 当前对象.调用(测试器, "workspace.create", map[string]any{"name": "命令", "root_path": 工作目录}), "id")
			}
			已受理 := 当前对象.调用(测试器, "conversation.send", 参数)
			要求成功(测试器, 已受理)
			执行标识 := 结果字符串(测试器, 已受理, "run_id")
			if 测试.决定 != "" {
				var 事件 struct {
					审批 智能体包.审批 `json:"approval"`
				}
				if 错误 := JSON包.反序列化(当前对象.事件(测试器, "approval.requested").参数, &事件); 错误 != nil {
					测试器.报告致命错误(错误)
				}
				var 操作 执行器包.操作
				if 错误 := JSON包.反序列化(事件.审批.请求, &操作); 错误 != nil {
					测试器.报告致命错误(错误)
				}
				var 详情 struct {
					可执行文件, 工作目录 string
					沙箱内         bool
				}
				if 错误 := JSON包.反序列化(操作.详情, &详情); 错误 != nil {
					测试器.报告致命错误(错误)
				}
				实际, 错误 := 文件路径包.求值符号链接集(工作目录)
				if 错误 != nil {
					测试器.报告致命错误(错误)
				}
				if !文件路径包.是否绝对路径(详情.可执行文件) || 详情.工作目录 != 实际 || 详情.沙箱内 != 沙箱内 || len(操作.工具.影响集) != 1 || 操作.工具.影响集[0] != 工具包.影响未知 {
					测试器.格式化报告致命错误("命令审批详情不完整: %s", 事件.审批.请求)
				}
				if _, 错误 := 操作系统包.状态信息(标记); !错误判断包.是否属于(错误, 操作系统包.不存在错误) {
					测试器.报告致命错误("审批前执行了命令")
				}
				if 测试.决定 == "cancel" {
					要求成功(测试器, 当前对象.调用(测试器, "run.cancel", map[string]any{"run_id": 执行标识}))
				} else {
					要求成功(测试器, 当前对象.调用(测试器, "approval.decide", map[string]any{"approval_id": 事件.审批.标识, "decision": 测试.决定}))
				}
			}
			var 已流式输出 字符串包.构建器
			if 测试.模式 == "stream" || 测试.模式 == "cancel" || 测试.模式 == "shutdown" {
				var 事件 struct {
					执行标识, 步骤标识 string
					输出         智能体包.工具输出 `json:"output"`
				}
				帧 := 当前对象.事件(测试器, "tool.output")
				var 字段集 map[string]JSON包.原始JSON消息
				if 错误 := JSON包.反序列化(帧.参数, &字段集); 错误 != nil {
					测试器.报告致命错误(错误)
				}
				if 错误 := JSON包.反序列化(字段集["run_id"], &事件.执行标识); 错误 != nil {
					测试器.报告致命错误(错误)
				}
				if 错误 := JSON包.反序列化(字段集["step_id"], &事件.步骤标识); 错误 != nil {
					测试器.报告致命错误(错误)
				}
				if 错误 := JSON包.反序列化(字段集["output"], &事件.输出); 错误 != nil {
					测试器.报告致命错误(错误)
				}
				字节, 错误 := 基准64包.标准编码.解码字符串(事件.输出.数据)
				if 错误 != nil {
					测试器.报告致命错误(错误)
				}
				已流式输出.写入(字节)
				if 事件.执行标识 != 执行标识 || 事件.步骤标识 == "" || 事件.输出.序号 != 1 || 事件.输出.编码 != "base64" {
					测试器.格式化报告致命错误("分片未关联执行: %s", 帧.参数)
				}
				switch 测试.模式 {
				case "stream":
					if 错误 := 操作系统包.写入文件(释放, nil, 0600); 错误 != nil {
						测试器.报告致命错误(错误)
					}
				case "cancel":
					要求成功(测试器, 当前对象.调用(测试器, "run.cancel", map[string]any{"run_id": 执行标识}))
				case "shutdown":
					当前对象.停止(测试器)
					之前, 错误 := 操作系统包.读取文件(心跳)
					if 错误 != nil {
						测试器.报告致命错误(错误)
					}
					计时器 := 时间包.新建计时器(120 * 时间包.毫秒)
					select {
					case <-计时器.字符:
					case <-测试器.上下文().完成():
						计时器.停止()
						测试器.报告致命错误(测试器.上下文().获取错误())
					}
					之后, 错误 := 操作系统包.读取文件(心跳)
					if 错误 != nil || string(之前) != string(之后) {
						测试器.报告致命错误("应用关闭后子进程继续写入")
					}
					当前对象 = 开始应用(测试器, 启动配置, 输入输出包.丢弃)
				}
			}
			if 测试.模式 != "shutdown" {
				终态 := "run.completed"
				if 测试.决定 == "cancel" || 测试.模式 == "cancel" {
					终态 = "run.cancelled"
				}
				当前对象.事件(测试器, 终态)
			}
			步骤, 原始 := 命令步骤输出(测试器, 当前对象, 执行标识)
			var 已保存 struct {
				操作 工具包.操作结果记录 `json:"operation"`
				结果 struct {
					边界   工具包.命令边界 `json:"execution_boundary"`
					已启动  bool
					退出码  *int `json:"exit_code"`
					已超时  bool `json:"timed_out"`
					标准输出 struct{ 数据 string }
				} `json:"result"`
			}
			if 错误 := JSON包.反序列化(原始, &已保存); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			预期 := 工具包.操作结果已生效
			if 测试.决定 == "rejected" || 测试.决定 == "cancel" || 测试.策略 == "never" && !自动 {
				预期 = 工具包.操作结果不已生效
			}
			if 测试.模式 == "cancel" || 测试.模式 == "timeout" || 测试.模式 == "shutdown" {
				预期 = 工具包.操作结果未知
			}
			if 已保存.操作.操作结果 != 预期 || 已保存.操作.请求哈希 == "" {
				测试器.格式化报告致命错误("终态没有保存事实: %s", 原始)
			}
			if 自动 && (!已保存.结果.边界.沙箱内 || 已保存.结果.边界.提权) {
				测试器.格式化报告致命错误("自动命令没有使用沙箱: %s", 原始)
			}
			if 预期 == 工具包.操作结果不已生效 {
				if 已保存.结果.已启动 {
					测试器.报告致命错误("被拒绝或取消后仍启动")
				}
				if _, 错误 := 操作系统包.状态信息(标记); !错误判断包.是否属于(错误, 操作系统包.不存在错误) {
					测试器.报告致命错误("未批准的命令产生副作用")
				}
			}
			if 删除 {
				if 数据, 错误 := 操作系统包.读取文件(标记); 错误 != nil || string(数据) != "preserved" || 已保存.结果.退出码 == nil || *已保存.结果.退出码 == 0 {
					测试器.报告致命错误("只读沙箱没有阻止删除")
				}
			}
			if 测试.模式 == "write" && 预期 == 工具包.操作结果已生效 {
				if 数据, 错误 := 操作系统包.读取文件(标记); 错误 != nil || string(数据) != "executed" {
					测试器.报告致命错误("命令未执行")
				}
			}
			if 测试.模式 == "stream" && (!字符串包.包含(已流式输出.字符串(), "first") || 已保存.结果.标准输出.数据 != "first中文\nsecond\n") {
				测试器.格式化报告致命错误("实时或最终输出丢失: %s", 原始)
			}
			if 测试.模式 == "exit" && (已保存.结果.退出码 == nil || *已保存.结果.退出码 != 7 || 步骤.状态 != 智能体包.已完成) {
				测试器.格式化报告致命错误("非零退出码被当作启动失败: %s", 原始)
			}
			if 测试.模式 == "timeout" && (!已保存.结果.已超时 || 步骤.状态 != 智能体包.失败) {
				测试器.格式化报告致命错误("超时记录错误: %s", 原始)
			}
			if 测试.模式 != "cancel" && 测试.模式 != "shutdown" && 测试.决定 != "cancel" && 结果集.加载() != 1 {
				测试器.格式化报告致命错误("模型未收到最终结果: %d", 结果集.加载())
			}
			当前对象.停止(测试器)
		})
	}
}

func Test默认命令无法读取应用凭据(测试器 *测试包.测试器) {
	if 运行时包.目标操作系统 != "darwin" {
		测试器.跳过("验证 macOS 自动命令的文件隔离")
	}
	const 测试夹具键 = "fixture-other-provider-private-key"
	启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	原始参数, 错误 := JSON包.序列化(map[string]any{
		"command": []string{"/usr/bin/sqlite3", "-readonly", 启动配置.数据库路径, "SELECT api_key FROM model_providers WHERE name='fixture-only'"},
		"cwd":     测试器.创建临时目录(),
	})
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	var 结果集 原子操作包.整数32
	上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
		var 输入 struct {
			工具集 []JSON包.原始JSON消息
			消息集 []struct{ 角色, 内容 string }
		}
		if 错误 := JSON包.新建解码器(当前请求.正文).解码(&输入); 错误 != nil || len(输入.消息集) == 0 {
			测试器.格式化创建错误("请求无效: %v", 错误)
			当前响应写入器.写入头部(HTTP包.状态无效请求)
			return
		}
		最后 := 输入.消息集[len(输入.消息集)-1]
		switch {
		case len(输入.工具集) == 0:
			写入文件工具响应(测试器, 当前响应写入器, false, map[string]any{"content": "权限核对"}, "stop")
		case 最后.角色 == "tool":
			if 字符串包.包含(最后.内容, 测试夹具键) {
				测试器.错误文本("凭据进入模型工具消息")
			}
			结果集.添加(1)
			写入文件工具响应(测试器, 当前响应写入器, false, map[string]any{"content": "核对完成"}, "stop")
		default:
			调用 := map[string]any{"id": "protected-db", "type": "function", "function": map[string]any{"name": "run_command", "arguments": string(原始参数)}}
			写入文件工具响应(测试器, 当前响应写入器, false, map[string]any{"tool_calls": []any{调用}}, "tool_calls")
		}
	}))
	defer 上游.关闭()
	当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	要求成功(测试器, 当前对象.调用(测试器, "provider.create", map[string]any{"name": "fixture-only", "protocol": "chatcompletions", "base_url": 上游.URL + "/v1", "api_key": 测试夹具键}))
	消息标识 := 初始化会话模型(测试器, 当前对象, 上游.URL)
	执行标识 := 结果字符串(测试器, 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "stream": false, "content": 消息内容块集("核对权限")}), "run_id")
	当前对象.事件(测试器, "run.completed")
	_, 原始 := 命令步骤输出(测试器, 当前对象, 执行标识)
	var 输出 struct {
		结果 struct {
			退出码 *int `json:"exit_code"`
		}
	}
	if 错误 := JSON包.反序列化(原始, &输出); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 输出.结果.退出码 == nil || *输出.结果.退出码 == 0 || 结果集.加载() != 1 || 字符串包.包含(string(原始), 测试夹具键) {
		测试器.格式化报告致命错误("应用数据库的隔离未生效: %s", 原始)
	}
	for _, 事件 := range 当前对象.待处理 {
		if 事件.方法 != "tool.output" {
			continue
		}
		var 分片 struct{ 输出 智能体包.工具输出 }
		if 错误 := JSON包.反序列化(事件.参数, &分片); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		数据, 错误 := 基准64包.标准编码.解码字符串(分片.输出.数据)
		if 错误 != nil || 字符串包.包含(string(数据), 测试夹具键) {
			测试器.报告致命错误("命令事件包含无效输出或凭据")
		}
	}
	当前对象.停止(测试器)
}
```
