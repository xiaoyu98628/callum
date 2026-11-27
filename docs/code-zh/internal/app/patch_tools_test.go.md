```go
package 应用模块

import (
	SHA256包 "crypto/sha256"
	十六进制包 "encoding/hex"
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
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	执行器包 "github.com/xiaoyu98628/callum/internal/tool/executor"
)

func Test补丁工具集通过标准输入输出(测试器 *测试包.测试器) {
	// 各用例独占临时目录、运行时及受控端点，可与其他应用回归并行。
	测试器.并行()
	for _, 测试 := range []struct {
		名称, 策略, 决定             string
		更新, 外部, 流, 变更期间审批, 分片的 bool
		附加                     bool
	}{
		{名称: "create_workspace"},
		{名称: "create_never", 策略: "never"},
		{名称: "update_stream", 更新: true, 流: true},
		{名称: "fragmented_create", 流: true, 分片的: true},
		{名称: "always_approve", 策略: "always", 决定: "approved"},
		{名称: "always_reject", 策略: "always", 决定: "rejected"},
		{名称: "outside_approve", 外部: true, 决定: "approved", 流: true},
		{名称: "additional_folder_approve", 外部: true, 附加: true, 决定: "approved"},
		{名称: "outside_never", 外部: true, 策略: "never"},
		{名称: "cancel_waiting", 策略: "always", 决定: "cancel"},
		{名称: "version_changed", 更新: true, 策略: "always", 决定: "approved", 变更期间审批: true},
	} {
		测试器.执行(测试.名称, func(测试器 *测试包.测试器) {
			工作区, 外部 := 测试器.创建临时目录(), 测试器.创建临时目录()
			根 := 工作区
			if 测试.外部 {
				根 = 外部
			}
			路径 := 文件路径包.合并(根, "note.txt")
			已解析根, 错误 := 文件路径包.求值符号链接集(根)
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			已解析路径 := 文件路径包.合并(已解析根, "note.txt")
			之前 := "head\nold\ntail\n"
			if 测试.更新 {
				if 错误 := 操作系统包.写入文件(路径, []byte(之前), 0640); 错误 != nil {
					测试器.报告致命错误(错误)
				}
			}
			var 工具结果集 原子操作包.整数32
			var 摘要 原子操作包.值
			部分已发送, 完成参数 := make(chan struct{}), make(chan struct{})
			上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
				var 请求 struct {
					流   bool             `json:"stream"`
					工具集 []JSON包.原始JSON消息 `json:"tools"`
					消息集 []struct {
						角色     string `json:"role"`
						内容     string `json:"content"`
						工具调用标识 string `json:"tool_call_id"`
					} `json:"messages"`
				}
				if 错误 := JSON包.新建解码器(当前请求.正文).解码(&请求); 错误 != nil || len(请求.消息集) == 0 {
					测试器.格式化创建错误("请求无效: %v", 错误)
					当前响应写入器.写入头部(400)
					return
				}
				if len(请求.工具集) != 9 {
					测试器.格式化创建错误("工具数量不符: %d", len(请求.工具集))
				}
				最后 := 请求.消息集[len(请求.消息集)-1]
				if 最后.角色 == "user" && 最后.内容 == "核对已写文件" {
					已找到 := false
					for _, 消息 := range 请求.消息集 {
						if 消息.角色 == "tool" && 消息.工具调用标识 == "patch-files" && 字符串包.包含(消息.内容, `"outcome":"applied"`) && 字符串包.包含(消息.内容, `"changes"`) {
							已找到 = true
						}
					}
					if !已找到 {
						测试器.错误文本("后续上下文没有恢复真实文件操作")
					}
					写入文件工具响应(测试器, 当前响应写入器, 请求.流, map[string]any{"content": "文件操作事实已恢复"}, "stop")
					return
				}
				if 最后.角色 == "tool" {
					var 结果 struct {
						操作 工具包.操作结果记录 `json:"operation"`
						结果 struct {
							SHA256 string `json:"sha256"`
						} `json:"result"`
						错误 *struct {
							类别 string `json:"kind"`
						} `json:"error"`
					}
					if 错误 := JSON包.反序列化([]byte(最后.内容), &结果); 错误 != nil {
						测试器.错误文本(错误)
						当前响应写入器.写入头部(400)
						return
					}
					if 最后.工具调用标识 == "read-before" {
						摘要.存储(结果.结果.SHA256)
					} else {
						工具结果集.添加(1)
						预期 := 工具包.操作结果已生效
						if 测试.决定 == "rejected" || (测试.外部 && 测试.策略 == "never") || 测试.变更期间审批 {
							预期 = 工具包.操作结果不已生效
						}
						if 结果.操作.操作结果 != 预期 {
							测试器.格式化创建错误("操作事实不符: %s", 最后.内容)
						}
						if 测试.变更期间审批 && (结果.错误 == nil || 结果.错误.类别 != "file_conflict") {
							测试器.格式化创建错误("没有返回版本冲突: %s", 最后.内容)
						}
						写入文件工具响应(测试器, 当前响应写入器, 请求.流, map[string]any{"content": "已记录文件操作结果"}, "stop")
						return
					}
				}
				调用路径 := "note.txt"
				if 测试.外部 {
					调用路径 = 路径
				}
				名称, 标识 := "apply_patch", "patch-files"
				var 参数 any
				if 测试.更新 && 摘要.加载() == nil {
					名称, 标识 = "read_file", "read-before"
					参数 = map[string]string{"path": 调用路径}
				} else {
					正文 := "*** Add File: " + 调用路径 + "\n+created\n"
					版本集 := []map[string]string{}
					if 测试.更新 {
						正文 = "*** Update File: " + 调用路径 + "\n@@\n head\n-old\n+new\n tail\n"
						版本集 = append(版本集, map[string]string{"path": 调用路径, "sha256": 摘要.加载().(string)})
					}
					参数 = map[string]any{"patch": "*** Begin Patch\n" + 正文 + "*** End Patch\n", "base_versions": 版本集}
				}
				原始, 错误 := JSON包.序列化(参数)
				if 错误 != nil {
					测试器.错误文本(错误)
					当前响应写入器.写入头部(500)
					return
				}
				调用 := map[string]any{"id": 标识, "type": "function", "function": map[string]any{"name": 名称, "arguments": string(原始)}}
				if 请求.流 {
					调用["index"] = 0
				}
				if 测试.分片的 && 名称 == "apply_patch" {
					截断 := len(原始) / 2
					调用["function"] = map[string]any{"name": 名称, "arguments": string(原始[:截断])}
					分片, 错误 := JSON包.序列化(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{调用}}, "finish_reason": nil}}})
					if 错误 != nil {
						测试器.错误文本(错误)
						return
					}
					当前响应写入器.头部().设置("Content-Type", "text/event-stream")
					if _, 错误 := 输入输出包.写入字符串(当前响应写入器, "data: "+string(分片)+"\n\n"); 错误 != nil {
						测试器.错误文本(错误)
						return
					}
					当前响应写入器.(HTTP包.刷新器).刷新()
					close(部分已发送)
					select {
					case <-完成参数:
					case <-当前请求.上下文().完成():
						return
					}
					写入文件工具响应(测试器, 当前响应写入器, true, map[string]any{"tool_calls": []any{map[string]any{"index": 0, "function": map[string]any{"arguments": string(原始[截断:])}}}}, "tool_calls")
					return
				}
				写入文件工具响应(测试器, 当前响应写入器, 请求.流, map[string]any{"tool_calls": []any{调用}}, "tool_calls")
			}))
			测试器.注册清理函数(上游.关闭)
			启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			启动配置.最大并发执行集 = 1
			当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
			if 测试.策略 != "" {
				设置审批策略(测试器, 当前对象, 测试.策略)
			}
			消息标识 := 初始化会话模型(测试器, 当前对象, 上游.URL)
			创建 := map[string]any{"name": "文件工具", "root_path": 工作区}
			if 测试.附加 {
				创建["other_paths"] = []string{外部}
			}
			工作区标识 := 结果字符串(测试器, 当前对象.调用(测试器, "workspace.create", 创建), "id")
			已受理 := 当前对象.调用(测试器, "conversation.send", map[string]any{"workspace_id": 工作区标识, "model_id": 消息标识, "stream": 测试.流, "content": 消息内容块集("创建或修改文件")})
			执行标识 := 结果字符串(测试器, 已受理, "run_id")
			if 测试.分片的 {
				select {
				case <-部分已发送:
				case <-时间包.之后(3 * 时间包.秒):
					测试器.报告致命错误("未收到流式参数片段")
				}
				if _, 错误 := 操作系统包.状态信息(路径); !操作系统包.是否不存在(错误) {
					测试器.报告致命错误("参数尚不完整时创建了文件")
				}
				close(完成参数)
			}
			var 补丁审批 智能体包.审批
			if 测试.决定 != "" {
				for {
					var 事件 struct {
						审批 智能体包.审批 `json:"approval"`
					}
					if 错误 := JSON包.反序列化(当前对象.事件(测试器, "approval.requested").参数, &事件); 错误 != nil {
						测试器.报告致命错误(错误)
					}
					var 操作 执行器包.操作
					if JSON包.反序列化(事件.审批.请求, &操作) != nil {
						测试器.报告致命错误("审批快照无效")
					}
					if 操作.工具.名称 == "read_file" {
						要求成功(测试器, 当前对象.调用(测试器, "approval.decide", map[string]any{"approval_id": 事件.审批.标识, "decision": "approved"}))
						continue
					}
					补丁审批 = 事件.审批
					if 操作.计划 == nil || len(操作.计划.目标集) != 1 || 操作.计划.目标集[0].路径 != 已解析路径 {
						测试器.格式化报告致命错误("审批没有固定实际写入目标: %s", 事件.审批.请求)
					}
					if 测试.更新 {
						校验和 := SHA256包.校验和256([]byte(之前))
						if 操作.计划.目标集[0].预期SHA256 != 十六进制包.编码目标字符串(校验和[:]) {
							测试器.报告致命错误("审批未绑定文件版本")
						}
						if 实际, 错误 := 操作系统包.读取文件(路径); 错误 != nil || string(实际) != 之前 {
							测试器.报告致命错误("审批前修改了目标")
						}
					} else if _, 错误 := 操作系统包.状态信息(路径); !操作系统包.是否不存在(错误) {
						测试器.报告致命错误("审批前创建了目标")
					}
					条目集, 错误 := 操作系统包.读取目录(根)
					if 错误 != nil {
						测试器.报告致命错误(错误)
					}
					for _, 条目 := range 条目集 {
						if 字符串包.存在前缀(条目.名称(), ".callum-patch-") {
							测试器.报告致命错误("审批前创建了恢复副本")
						}
					}
					if 测试.变更期间审批 {
						if 错误 := 操作系统包.写入文件(路径, []byte("user changed"), 0640); 错误 != nil {
							测试器.报告致命错误(错误)
						}
					}
					if 测试.决定 == "cancel" {
						要求成功(测试器, 当前对象.调用(测试器, "run.cancel", map[string]any{"run_id": 执行标识}))
					} else {
						要求成功(测试器, 当前对象.调用(测试器, "approval.decide", map[string]any{"approval_id": 事件.审批.标识, "decision": 测试.决定}))
					}
					break
				}
			}
			终态 := "run.completed"
			if 测试.决定 == "cancel" {
				终态 = "run.cancelled"
			}
			当前对象.事件(测试器, 终态)
			预期 := !测试.变更期间审批 && 测试.决定 != "rejected" && 测试.决定 != "cancel" && !(测试.外部 && 测试.策略 == "never")
			实际, 读取错误 := 操作系统包.读取文件(路径)
			if 预期 {
				文本 := "created\n"
				if 测试.更新 {
					文本 = "head\nnew\ntail\n"
				}
				if 读取错误 != nil || string(实际) != 文本 {
					测试器.格式化报告致命错误("实际文件内容不符: %q %v", 实际, 读取错误)
				}
			} else if 测试.更新 {
				文本 := 之前
				if 测试.变更期间审批 {
					文本 = "user changed"
				}
				if 读取错误 != nil || string(实际) != 文本 {
					测试器.报告致命错误("覆盖了用户文件")
				}
			} else if !操作系统包.是否不存在(读取错误) {
				测试器.报告致命错误("被拒绝或取消的操作创建了文件")
			}
			步骤集帧 := 当前对象.调用(测试器, "step.list", map[string]any{"run_id": 执行标识})
			要求成功(测试器, 步骤集帧)
			var 步骤集 []智能体包.步骤摘要
			if 错误 := JSON包.反序列化(步骤集帧.结果, &步骤集); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			已找到 := false
			for _, 步骤 := range 步骤集 {
				if 步骤.名称 != "apply_patch" {
					continue
				}
				已找到 = true
				帧 := 当前对象.调用(测试器, "step.data", map[string]any{"step_id": 步骤.标识, "field": "output"})
				要求成功(测试器, 帧)
				var 数据 struct {
					片段 string `json:"fragment"`
				}
				if JSON包.反序列化(帧.结果, &数据) != nil || !字符串包.包含(数据.片段, `"operation"`) {
					测试器.报告致命错误("步骤缺少操作事实")
				}
			}
			if !已找到 {
				测试器.报告致命错误("缺少持久化补丁步骤")
			}
			if 预期 {
				会话标识 := 结果字符串(测试器, 已受理, "conversation_id")
				要求成功(测试器, 当前对象.调用(测试器, "conversation.send", map[string]any{"conversation_id": 会话标识, "stream": 测试.流, "content": 消息内容块集("核对已写文件")}))
				当前对象.事件(测试器, "run.completed")
			}
			if 测试.决定 != "cancel" && 工具结果集.加载() != 1 {
				测试器.格式化报告致命错误("模型未收到结果: %d", 工具结果集.加载())
			}
			if 补丁审批.标识 != "" && 测试.决定 == "cancel" {
				帧 := 当前对象.调用(测试器, "approval.get", map[string]any{"id": 补丁审批.标识})
				要求成功(测试器, 帧)
				if !字符串包.包含(string(帧.结果), "cancelled") {
					测试器.报告致命错误("取消未结束审批")
				}
			}
			当前对象.停止(测试器)
		})
	}
}
```
