```go
package 应用模块

import (
	字节包 "bytes"
	JSON包 "encoding/json"
	输入输出包 "io"
	HTTP包 "net/http"
	HTTP测试包 "net/http/httptest"
	操作系统包 "os"
	文件路径包 "path/filepath"
	原子操作包 "sync/atomic"
	测试包 "testing"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	配置模块包 "github.com/xiaoyu98628/callum/internal/config"
	执行器包 "github.com/xiaoyu98628/callum/internal/tool/executor"
)

func Test文件工具集独立所属工作区通过标准输入输出(测试器 *测试包.测试器) {
	// 各用例独占临时目录、运行时及受控端点，可与其他应用回归并行。
	测试器.并行()
	for _, 测试 := range []struct {
		名称, 操作, 策略, 决定, 错误类别    string
		边界, 内部, 相对, 符号链接, 替换, 流 bool
		能力集                     map[string]bool
		模型类型集                   []string
		附加                      bool
	}{
		{名称: "unbound_automatic_stream", 流: true},
		{名称: "tools_false_does_not_disable_tools", 能力集: map[string]bool{"tools": false}, 模型类型集: []string{"text"}},
		{名称: "content_types_do_not_disable_tools", 能力集: map[string]bool{"reasoning": true}, 模型类型集: []string{"text", "image", "audio", "video"}},
		{名称: "unbound_always_reject", 策略: "always", 决定: "rejected", 错误类别: "permission_denied"},
		{名称: "unbound_always_cancel", 策略: "always", 决定: "cancel"},
		{名称: "unbound_never", 策略: "never"},
		{名称: "unbound_relative", 相对: true, 错误类别: "path_base_required"},
		{名称: "workspace_absolute_inside", 边界: true, 内部: true},
		{名称: "workspace_absolute_inside_never", 边界: true, 内部: true, 策略: "never"},
		{名称: "workspace_absolute_inside_always", 边界: true, 内部: true, 策略: "always", 决定: "approved"},
		{名称: "workspace_absolute_outside", 边界: true},
		{名称: "workspace_additional_absolute", 边界: true, 附加: true},
		{名称: "workspace_main_relative_with_additional", 边界: true, 内部: true, 相对: true, 附加: true},
		{名称: "workspace_outside_never", 边界: true, 策略: "never"},
		{名称: "workspace_symlink_outside", 边界: true, 符号链接: true, 相对: true},
		{名称: "unbound_directory", 操作: "list_directory"},
		{名称: "unbound_directory_never", 操作: "list_directory", 策略: "never"},
		{名称: "outside_directory", 操作: "list_directory", 边界: true},
		{名称: "unbound_search", 操作: "search_text"},
		{名称: "unbound_search_never", 操作: "search_text", 策略: "never"},
		{名称: "outside_search", 操作: "search_text", 边界: true},
		{名称: "unbound_target_replaced", 策略: "always", 决定: "approved", 替换: true, 错误类别: "permission_denied"},
	} {
		测试器.执行(测试.名称, func(测试器 *测试包.测试器) {
			工作区根, 外部 := 测试器.创建临时目录(), 测试器.创建临时目录()
			实际根, 错误 := 文件路径包.求值符号链接集(工作区根)
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			实际其他, 错误 := 文件路径包.求值符号链接集(外部)
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			根 := 外部
			if 测试.内部 {
				根 = 工作区根
			}
			文件 := 文件路径包.合并(根, "note.txt")
			if 错误 := 操作系统包.写入文件(文件, []byte("actual-file-content"), 0600); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			操作 := 测试.操作
			if 操作 == "" {
				操作 = "read_file"
			}
			路径 := 文件
			if 操作 != "read_file" {
				路径 = 根
			}
			if 测试.符号链接 {
				if 错误 := 操作系统包.符号链接(文件, 文件路径包.合并(工作区根, "note.txt")); 错误 != nil {
					测试器.报告致命错误(错误)
				}
			}
			if 测试.相对 {
				路径 = "note.txt"
			}
			参数 := map[string]any{"path": 路径}
			if 操作 == "search_text" {
				参数["query"] = "actual-file-content"
			}
			参数, 错误 := JSON包.序列化(参数)
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			var 结果集 原子操作包.整数32
			上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
				var 请求 struct {
					流   bool                      `json:"stream"`
					工具集 []JSON包.原始JSON消息          `json:"tools"`
					消息集 []struct{ 角色, 内容 string } `json:"messages"`
				}
				if 错误 := JSON包.新建解码器(当前请求.正文).解码(&请求); 错误 != nil || len(请求.消息集) == 0 {
					测试器.格式化创建错误("请求无效: %v", 错误)
					当前响应写入器.写入头部(400)
					return
				}
				if len(请求.工具集) != 9 {
					测试器.格式化创建错误("未绑定工作区时丢失工具定义: %d", len(请求.工具集))
				}
				if 测试.附加 {
					已找到 := false
					for _, 消息 := range 请求.消息集 {
						if 消息.角色 == "user" && 字节包.包含([]byte(消息.内容), []byte(实际根)) &&
							字节包.包含([]byte(消息.内容), []byte(实际其他)) {
							已找到 = true
						}
					}
					if !已找到 {
						测试器.错误文本("模型请求缺少主目录和其他目录")
					}
				}
				最后 := 请求.消息集[len(请求.消息集)-1]
				if 最后.角色 == "tool" {
					结果集.添加(1)
					var 结果 struct {
						错误 *struct {
							类别 string `json:"kind"`
						} `json:"error"`
					}
					if 错误 := JSON包.反序列化([]byte(最后.内容), &结果); 错误 != nil {
						测试器.错误文本(错误)
					}
					if 测试.错误类别 != "" {
						if 结果.错误 == nil || 结果.错误.类别 != 测试.错误类别 {
							测试器.格式化创建错误("错误结果不符: %s", 最后.内容)
						}
					} else if 结果.错误 != nil {
						测试器.格式化创建错误("文件操作失败: %s", 最后.内容)
					} else if 操作 == "read_file" || 操作 == "search_text" {
						if !字节包.包含([]byte(最后.内容), []byte("actual-file-content")) {
							测试器.格式化创建错误("没有返回实际内容: %s", 最后.内容)
						}
					} else if !字节包.包含([]byte(最后.内容), []byte("note.txt")) {
						测试器.格式化创建错误("没有返回目录条目: %s", 最后.内容)
					}
					写入文件工具响应(测试器, 当前响应写入器, 请求.流, map[string]any{"content": "读取完成"}, "stop")
					return
				}
				调用 := map[string]any{"id": "call-file", "type": "function", "function": map[string]any{"name": 操作, "arguments": string(参数)}}
				if 请求.流 {
					调用["index"] = 0
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
			if 测试.能力集 != nil || 测试.模型类型集 != nil {
				帧 := 当前对象.调用(测试器, "model.get", map[string]any{"id": 消息标识})
				要求成功(测试器, 帧)
				var 已有 struct {
					供应商标识 string `json:"provider_id"`
				}
				if 错误 := JSON包.反序列化(帧.结果, &已有); 错误 != nil {
					测试器.报告致命错误(错误)
				}
				要求成功(测试器, 当前对象.调用(测试器, "model.update", map[string]any{"id": 消息标识, "provider_id": 已有.供应商标识, "display_name": "类型元数据", "remote_model_id": "remote-model", "enabled": true, "context_window": 1000000, "capabilities": 测试.能力集, "model_type": 测试.模型类型集}))
			}
			参数 := map[string]any{"model_id": 消息标识, "stream": 测试.流, "content": 消息内容块集("读取指定目标")}
			if 测试.边界 {
				创建 := map[string]any{"name": "项目", "root_path": 工作区根}
				if 测试.附加 {
					创建["other_paths"] = []string{外部}
				}
				参数["workspace_id"] = 结果字符串(测试器, 当前对象.调用(测试器, "workspace.create", 创建), "id")
			}
			已受理 := 当前对象.调用(测试器, "conversation.send", 参数)
			执行标识 := 结果字符串(测试器, 已受理, "run_id")
			var 审批标识 string
			if 测试.决定 != "" {
				var 事件 struct {
					审批 智能体包.审批 `json:"approval"`
				}
				if 错误 := JSON包.反序列化(当前对象.事件(测试器, "approval.requested").参数, &事件); 错误 != nil {
					测试器.报告致命错误(错误)
				}
				审批标识 = 事件.审批.标识
				var 快照 执行器包.操作
				if 错误 := JSON包.反序列化(事件.审批.请求, &快照); 错误 != nil {
					测试器.报告致命错误(错误)
				}
				已解析, 错误 := 文件路径包.求值符号链接集(文件)
				if 操作 != "read_file" {
					已解析, 错误 = 文件路径包.求值符号链接集(根)
				}
				if 错误 != nil {
					测试器.报告致命错误(错误)
				}
				if 快照.目标 == nil || 快照.目标.路径 != 已解析 || 快照.目标.范围内工作区 != 测试.内部 || (快照.工作区 != nil) != 测试.边界 {
					测试器.格式化报告致命错误("审批目标不准确: %s", 事件.审批.请求)
				}
				if 结果集.加载() != 0 {
					测试器.报告致命错误("审批前已返回工具结果")
				}
				if 测试.替换 {
					if 错误 := 操作系统包.重命名(文件, 文件+"-original"); 错误 != nil {
						测试器.报告致命错误(错误)
					}
					if 错误 := 操作系统包.写入文件(文件, []byte("unapproved replacement"), 0600); 错误 != nil {
						测试器.报告致命错误(错误)
					}
				}
				if 测试.决定 == "cancel" {
					要求成功(测试器, 当前对象.调用(测试器, "run.cancel", map[string]any{"run_id": 执行标识}))
				} else {
					要求成功(测试器, 当前对象.调用(测试器, "approval.decide", map[string]any{"approval_id": 审批标识, "decision": 测试.决定}))
				}
			}
			终态 := "run.completed"
			if 测试.决定 == "cancel" {
				终态 = "run.cancelled"
			}
			当前对象.事件(测试器, 终态)
			预期结果集 := int32(1)
			if 测试.决定 == "cancel" {
				预期结果集 = 0
			}
			if 结果集.加载() != 预期结果集 {
				测试器.格式化报告致命错误("工具结果次数错误: %d", 结果集.加载())
			}
			帧 := 当前对象.调用(测试器, "approval.list", map[string]any{"run_id": 执行标识})
			要求成功(测试器, 帧)
			var 行集 []智能体包.审批
			if 错误 := JSON包.反序列化(帧.结果, &行集); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			预期 := 0
			if 审批标识 != "" {
				预期 = 1
			}
			if len(行集) != 预期 {
				测试器.格式化报告致命错误("审批数量错误: %s", 帧.结果)
			}
			if 测试.决定 == "cancel" && 行集[0].状态 != 智能体包.审批已取消 {
				测试器.报告致命错误("取消没有结束待审批记录")
			}
			当前对象.停止(测试器)
		})
	}
}

// 写入文件工具响应 用同一响应内容覆盖 JSON 与完整 SSE 工具参数的真实适配调用链。
func 写入文件工具响应(测试器 *测试包.测试器, 当前响应写入器 HTTP包.响应写入器, 流 bool, 内容 map[string]any, 原因 string) {
	测试器.标记测试辅助函数()
	选项 := map[string]any{"index": 0, "finish_reason": 原因}
	if 流 {
		选项["delta"] = 内容
		当前响应写入器.头部().设置("Content-Type", "text/event-stream")
	} else {
		内容["role"] = "assistant"
		选项["message"] = 内容
		当前响应写入器.头部().设置("Content-Type", "application/json")
	}
	数据, 错误 := JSON包.序列化(map[string]any{"choices": []any{选项}, "usage": map[string]int{"total_tokens": 1}})
	if 错误 != nil {
		测试器.错误文本(错误)
		return
	}
	if 流 {
		_, 错误 = 输入输出包.写入字符串(当前响应写入器, "data: "+string(数据)+"\n\ndata: [DONE]\n\n")
	} else {
		_, 错误 = 当前响应写入器.写入(数据)
	}
	if 错误 != nil {
		测试器.错误文本(错误)
	}
}
```
