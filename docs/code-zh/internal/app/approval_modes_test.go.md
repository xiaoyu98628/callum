```go
package 应用模块

import (
	JSON包 "encoding/json"
	输入输出包 "io"
	HTTP包 "net/http"
	HTTP测试包 "net/http/httptest"
	操作系统包 "os"
	文件路径包 "path/filepath"
	运行时包 "runtime"
	字符串包 "strings"
	原子操作包 "sync/atomic"
	测试包 "testing"
	时间包 "time"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	配置模块包 "github.com/xiaoyu98628/callum/internal/config"
	设置包 "github.com/xiaoyu98628/callum/internal/settings"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	策略包 "github.com/xiaoyu98628/callum/internal/tool/policy"
)

func 保存审批模式(测试器 *测试包.测试器, 当前应用通信对端 *应用通信对端, 执行 设置包.执行) {
	测试器.标记测试辅助函数()
	要求成功(测试器, 当前应用通信对端.调用(测试器, "settings.update", map[string]any{"key": 设置包.执行键, "revision": 读取设置(测试器, 当前应用通信对端).修订号, "value": 执行}))
}

func Test辅助审批通过标准输入输出(测试器 *测试包.测试器) {
	for _, 测试用例 := range []struct {
		名称, 回复       string
		审核状态         string
		已取消, 高风险, 超时 bool
	}{
		{名称: "approve", 回复: `{"decision":"approved","reason":"用户明确要求创建此文件。"}`, 审核状态: 智能体包.审批已批准},
		{名称: "reject", 回复: `{"decision":"rejected","reason":"与请求无关。"}`, 审核状态: 智能体包.审批已拒绝},
		{名称: "handoff", 回复: `{"decision":"handoff","reason":"需要核对目标。"}`, 审核状态: "handoff"},
		{名称: "malformed", 回复: `{"decision":"rejected","decision":"approved","reason":"重复键"}`, 审核状态: "handoff"},
		{名称: "provider_error", 审核状态: "handoff"},
		{名称: "cancel", 已取消: true},
		{名称: "timeout", 超时: true, 审核状态: "handoff"},
		{名称: "escalated_command", 高风险: true},
		{名称: "workspace_script", 高风险: true},
		{名称: "workspace_mkdir", 回复: `{"decision":"approved","reason":"用户明确要求创建目录。"}`, 审核状态: 智能体包.审批已批准},
	} {
		测试器.执行(测试用例.名称, func(测试器 *测试包.测试器) {
			创建目录 := 测试用例.名称 == "workspace_mkdir"
			脚本 := 测试用例.名称 == "workspace_script"
			if (创建目录 || 脚本) && 运行时包.目标操作系统 != "darwin" {
				测试器.跳过("验证 Seatbelt 项目写入")
			}
			根 := 测试器.创建临时目录()
			标记 := 文件路径包.合并(根, "reviewed.txt")
			沙箱, 意图 := 工具包.沙箱只读, "请创建 reviewed.txt 并写入指定文字。"
			工具名称 := "apply_patch"
			参数 := map[string]any{"patch": "*** Begin Patch\n*** Add File: reviewed.txt\n+approved only\n*** End Patch\n", "base_versions": []any{}}
			switch {
			case 脚本:
				工具名称, 沙箱 = "run_command", 工具包.沙箱工作区写入
				参数 = map[string]any{"command": []string{"/bin/sh", "-c", `printf "approved only\n" > reviewed.txt`}, "sandbox_permissions": "workspace_write", "justification": "在沙箱内执行测试脚本"}
			case 测试用例.高风险:
				工具名称 = "run_command"
				参数 = map[string]any{"command": []string{"/usr/bin/touch", 标记}, "sandbox_permissions": "require_escalated", "justification": "受控审批验证"}
			case 创建目录:
				工具名称, 沙箱, 意图 = "run_command", 工具包.沙箱工作区写入, "请创建 reviewed-dir 目录。"
				标记 = 文件路径包.合并(根, "reviewed-dir")
				参数 = map[string]any{"command": []string{"/bin/mkdir", "reviewed-dir"}, "sandbox_permissions": "workspace_write", "justification": "在项目内创建测试目录"}
			}
			原始参数, 错误 := JSON包.序列化(参数)
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			var 审核调用集 原子操作包.整数32
			已开始 := make(chan struct{})
			上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
				var 请求 struct {
					工具集 []JSON包.原始JSON消息
					流   bool
					消息集 []struct{ 角色, 内容 string }
				}
				if 错误 := JSON包.新建解码器(当前请求.正文).解码(&请求); 错误 != nil || len(请求.消息集) == 0 {
					测试器.格式化创建错误("无效请求: %v", 错误)
					当前响应写入器.写入头部(400)
					return
				}
				if 字符串包.存在前缀(请求.消息集[0].内容, "你独立审核") {
					审核调用集.添加(1)
					if 请求.流 || len(请求.工具集) != 0 || len(请求.消息集) != 2 || !字符串包.包含(请求.消息集[1].内容, 意图) || !字符串包.包含(请求.消息集[1].内容, "fixed_operation") {
						测试器.格式化创建错误("审核上下文没有隔离: %+v", 请求)
					}
					if 测试用例.已取消 || 测试用例.超时 {
						close(已开始)
						<-当前请求.上下文().完成()
						return
					}
					if 测试用例.回复 == "" {
						当前响应写入器.写入头部(503)
						return
					}
					当前响应写入器.头部().设置("Content-Type", "application/json")
					_ = JSON包.新建编码器(当前响应写入器).编码(map[string]any{"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": 测试用例.回复}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 42, "completion_tokens": 5, "total_tokens": 47}})
					return
				}
				if len(请求.工具集) == 0 || 请求.消息集[len(请求.消息集)-1].角色 == "tool" {
					写入文件工具响应(测试器, 当前响应写入器, 请求.流, map[string]any{"content": "任务结束"}, "stop")
					return
				}
				调用 := map[string]any{"id": "review-target", "type": "function", "function": map[string]any{"name": 工具名称, "arguments": string(原始参数)}}
				写入文件工具响应(测试器, 当前响应写入器, 请求.流, map[string]any{"tool_calls": []any{调用}}, "tool_calls")
			}))
			defer 上游.关闭()
			启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			启动配置.最大并发执行集 = 1
			if 测试用例.超时 {
				启动配置.调用超时 = 400 * 时间包.毫秒
			}
			当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
			defer func() {
				if !当前对象.已停止 {
					当前对象.停止(测试器)
				}
			}()
			保存审批模式(测试器, 当前对象, 设置包.执行{审批策略: 策略包.开启请求, 审批审核方: 策略包.自动审核, 沙箱模式: 沙箱})
			消息标识 := 初始化会话模型(测试器, 当前对象, 上游.URL)
			工作区标识 := 结果字符串(测试器, 当前对象.调用(测试器, "workspace.create", map[string]any{"name": "受控审核项目", "root_path": 根}), "id")
			已受理 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "workspace_id": 工作区标识, "content": 消息内容块集(意图), "stream": false})
			执行标识 := 结果字符串(测试器, 已受理, "run_id")
			if 测试用例.已取消 {
				select {
				case <-已开始:
				case <-时间包.之后(5 * 时间包.秒):
					测试器.报告致命错误("审核未开始")
				}
				要求成功(测试器, 当前对象.调用(测试器, "run.cancel", map[string]any{"run_id": 执行标识}))
				当前对象.事件(测试器, "run.cancelled")
			} else if 测试用例.高风险 || 测试用例.审核状态 == "handoff" {
				var 事件 struct{ 审批 智能体包.审批 }
				if 错误 := JSON包.反序列化(当前对象.事件(测试器, "approval.requested").参数, &事件); 错误 != nil {
					测试器.报告致命错误(错误)
				}
				if !测试用例.高风险 && (事件.审批.审核 == nil || 事件.审批.审核.决定 != "handoff") {
					测试器.格式化报告致命错误("未保留转人工的审核结论: %+v", 事件.审批)
				}
				要求成功(测试器, 当前对象.调用(测试器, "approval.decide", map[string]any{"approval_id": 事件.审批.标识, "decision": "rejected"}))
				当前对象.事件(测试器, "run.completed")
			} else {
				var 事件 struct{ 审批 智能体包.审批 }
				if 错误 := JSON包.反序列化(当前对象.事件(测试器, "approval.updated").参数, &事件); 错误 != nil {
					测试器.报告致命错误(错误)
				}
				甲 := 事件.审批
				if 甲.已决定依据 == nil || *甲.已决定依据 != 智能体包.审批自动审核方 || 甲.状态 != 测试用例.审核状态 || 甲.审核 == nil || 甲.目录授权 != nil || 甲.目录范围 != nil {
					测试器.格式化报告致命错误("自动决定未绑定审批记录: %+v", 甲)
				}
				当前对象.事件(测试器, "run.completed")
				// 已提交的 AI 决定不会被迟到的人工决定覆盖或认领。
				if 实际值 := 当前对象.调用(测试器, "approval.decide", map[string]any{"approval_id": 甲.标识, "decision": 甲.状态}); 实际值.错误 == nil || 实际值.错误.代码 != 1003 {
					测试器.报告致命错误("人工请求接管了已决定审批")
				}
				步骤集 := 当前对象.调用(测试器, "step.list", map[string]any{"run_id": 执行标识})
				要求成功(测试器, 步骤集)
				var 行集 []智能体包.步骤摘要
				if 错误 := JSON包.反序列化(步骤集.结果, &行集); 错误 != nil {
					测试器.报告致命错误(错误)
				}
				已审计 := false
				for _, 行 := range 行集 {
					if 行.标识 == 甲.审核.步骤标识 && 行.名称 == "AI 审批" && 行.模型 != nil && 行.模型.标识 == 消息标识 && 行.用量字节 > 0 {
						已审计 = true
					}
				}
				if !已审计 {
					测试器.报告致命错误("审核模型或实际用量未记录")
				}
				if 测试用例.审核状态 == 智能体包.审批已批准 {
					当前对象.停止(测试器)
					当前对象 = 开始应用(测试器, 启动配置, 输入输出包.丢弃)
					已存储 := 当前对象.调用(测试器, "approval.get", map[string]any{"id": 甲.标识})
					要求成功(测试器, 已存储)
					var 已恢复 智能体包.审批
					if 错误 := JSON包.反序列化(已存储.结果, &已恢复); 错误 != nil || 已恢复.已决定依据 == nil || *已恢复.已决定依据 != 智能体包.审批自动审核方 || 已恢复.审核 == nil || *已恢复.审核 != *甲.审核 {
						测试器.格式化报告致命错误("重启后审核记录不一致: %+v %v", 已恢复, 错误)
					}
				}
			}
			预期调用集 := int32(1)
			if 测试用例.高风险 {
				预期调用集 = 0
			}
			if 测试用例.回复 != "" {
				var 步骤集 []智能体包.步骤摘要
				文件 := 当前对象.调用(测试器, "step.list", map[string]any{"run_id": 执行标识})
				要求成功(测试器, 文件)
				if 错误 := JSON包.反序列化(文件.结果, &步骤集); 错误 != nil {
					测试器.报告致命错误(错误)
				}
				已找到 := false
				for _, 步骤 := range 步骤集 {
					if 步骤.名称 != "AI 审批" {
						continue
					}
					数据 := 当前对象.调用(测试器, "step.data", map[string]any{"step_id": 步骤.标识, "field": "output"})
					要求成功(测试器, 数据)
					var 片段 struct{ 片段 string }
					if 错误 := JSON包.反序列化(数据.结果, &片段); 错误 != nil {
						测试器.报告致命错误(错误)
					}
					var 输出 struct {
						响应 struct {
							选项集 []struct{ 消息 struct{ 内容 string } }
						}
						审核 *智能体包.审批审核 `json:"approval_review"`
					}
					if JSON包.反序列化([]byte(片段.片段), &输出) != nil || len(输出.响应.选项集) != 1 || 输出.响应.选项集[0].消息.内容 != 测试用例.回复 || 输出.审核 == nil {
						测试器.报告致命错误("审核步骤未保留原始响应及独立结论")
					}
					已找到 = true
				}
				if !已找到 {
					测试器.报告致命错误("缺少审核审计步骤")
				}
			}
			if 审核调用集.加载() != 预期调用集 {
				测试器.格式化报告致命错误("不符合审核范围: %d", 审核调用集.加载())
			}
			if 创建目录 {
				if 信息, 错误 := 操作系统包.状态信息(标记); 错误 != nil || !信息.是否目录() {
					测试器.格式化报告致命错误("获批的沙箱目录创建未执行: %v", 错误)
				}
				return
			}
			数据, 错误 := 操作系统包.读取文件(标记)
			if 测试用例.审核状态 == 智能体包.审批已批准 {
				if 错误 != nil || string(数据) != "approved only\n" {
					测试器.格式化报告致命错误("获批操作未执行: %s %v", 数据, 错误)
				}
			} else if !操作系统包.是否不存在(错误) {
				测试器.格式化报告致命错误("非批准操作已写入: %q %v", 数据, 错误)
			}
		})
	}
}

func Test完整访问通过标准输入输出(测试器 *测试包.测试器) {
	if 运行时包.目标操作系统 == "windows" {
		测试器.跳过("测试命令使用 Unix shell")
	}
	根, 外部 := 测试器.创建临时目录(), 测试器.创建临时目录()
	标记 := 文件路径包.合并(外部, "full-access.txt")
	参数, 错误 := JSON包.序列化(map[string]any{"command": []string{"/bin/sh", "-c", `printf approved > "$1"`, "sh", 标记}})
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	服务端 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
		var 请求 struct {
			工具集 []JSON包.原始JSON消息
			流   bool
			消息集 []struct{ 角色 string }
		}
		if 错误 := JSON包.新建解码器(当前请求.正文).解码(&请求); 错误 != nil || len(请求.消息集) == 0 {
			当前响应写入器.写入头部(400)
			return
		}
		if len(请求.工具集) == 0 || 请求.消息集[len(请求.消息集)-1].角色 == "tool" {
			写入文件工具响应(测试器, 当前响应写入器, 请求.流, map[string]any{"content": "已结束"}, "stop")
			return
		}
		调用 := map[string]any{"id": "full-target", "type": "function", "function": map[string]any{"name": "run_command", "arguments": string(参数)}}
		写入文件工具响应(测试器, 当前响应写入器, 请求.流, map[string]any{"tool_calls": []any{调用}}, "tool_calls")
	}))
	defer 服务端.关闭()
	启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	defer 当前对象.停止(测试器)
	保存审批模式(测试器, 当前对象, 设置包.执行{审批策略: 策略包.从不, 审批审核方: 策略包.用户, 沙箱模式: 工具包.沙箱完整访问})
	消息标识 := 初始化会话模型(测试器, 当前对象, 服务端.URL)
	工作区标识 := 结果字符串(测试器, 当前对象.调用(测试器, "workspace.create", map[string]any{"name": "完全访问测试", "root_path": 根}), "id")
	已发送 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "workspace_id": 工作区标识, "stream": false, "content": 消息内容块集("写入隔离的外部目录")})
	执行标识 := 结果字符串(测试器, 已发送, "run_id")
	当前对象.事件(测试器, "run.completed")
	审批集 := 当前对象.调用(测试器, "approval.list", map[string]any{"run_id": 执行标识})
	要求成功(测试器, 审批集)
	if string(审批集.结果) != "[]" {
		测试器.格式化报告致命错误("完全访问意外等待审批: %s", 审批集.结果)
	}
	数据, 错误 := 操作系统包.读取文件(标记)
	if 错误 != nil || string(数据) != "approved" {
		测试器.格式化报告致命错误("未以用户权限写入隔离目录: %q %v", 数据, 错误)
	}
	_, 原始 := 命令步骤输出(测试器, 当前对象, 执行标识)
	if !字符串包.包含(string(原始), `"sandbox_mode":"danger-full-access"`) || !字符串包.包含(string(原始), `"sandboxed":false`) {
		测试器.格式化报告致命错误("命令边界未正确记录: %s", 原始)
	}
}
```
