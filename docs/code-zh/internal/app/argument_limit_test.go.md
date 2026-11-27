```go
package 应用模块

import (
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

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	配置模块包 "github.com/xiaoyu98628/callum/internal/config"
)

func Test超限参数正确之前执行中批次(测试器 *测试包.测试器) {
	测试器.并行()
	for _, 流 := range []bool{false, true} {
		测试器.执行(格式化包.转为字符串(流), func(测试器 *测试包.测试器) {
			根 := 测试器.创建临时目录()
			从不, 首个, 秒 := 文件路径包.合并(根, "never.txt"), 文件路径包.合并(根, "one.txt"), 文件路径包.合并(根, "two.txt")
			补丁 := func(路径, 文本 string) string {
				return 纠正JSON(测试器, map[string]any{"patch": "*** Begin Patch\n*** Add File: " + 文件路径包.目标斜杠(路径) + "\n+" + 文本 + "\n*** End Patch\n", "base_versions": []any{}})
			}
			var 调用集 原子操作包.整数32
			上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
				var 输入 struct {
					流   bool
					工具集 []JSON包.原始JSON消息
					消息集 []struct{ 角色, 内容 string }
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
				数量 := 调用集.添加(1)
				if 数量 == 1 {
					消息 := map[string]any{"content": "准备生成全文文件", "tool_calls": []any{
						纠正调用("valid-but-not-executed", "apply_patch", 补丁(从不, "never"), 输入.流, 0),
						纠正调用("large", "apply_patch", 补丁(文件路径包.合并(根, "huge.txt"), 字符串包.重复("中", 5500)), 输入.流, 1),
					}}
					if 输入.流 {
						// 复现实际故障，参数超限时尚未收到 结束原因，关闭读取后才结束发送。
						当前响应写入器.头部().设置("Content-Type", "text/event-stream")
						分片 := map[string]any{"choices": []any{map[string]any{"index": 0, "delta": 消息}}}
						_, _ = 格式化包.格式化输出(当前响应写入器, "data: %s\n\n", 纠正JSON(测试器, 分片))
						当前响应写入器.(HTTP包.刷新器).刷新()
						<-当前请求.上下文().完成()
					} else {
						写入文件工具响应(测试器, 当前响应写入器, false, 消息, "tool_calls")
					}
					return
				}
				if 数量 == 2 {
					反馈 := ""
					for _, 消息 := range 输入.消息集 {
						if 字符串包.包含(消息.内容, "服务端工具参数校验反馈") {
							反馈 = 消息.内容
						}
					}
					if !字符串包.包含(反馈, "16384") || !字符串包.包含(反馈, "apply_patch") || !字符串包.包含(反馈, "均未执行") {
						测试器.格式化创建错误("缺少可操作的超限反馈: %q", 反馈)
					}
					if _, 错误 := 操作系统包.状态信息(从不); !操作系统包.是否不存在(错误) {
						测试器.错误文本("执行了超限批次中的有效调用")
					}
					写入文件工具响应(测试器, 当前响应写入器, 输入.流, map[string]any{"tool_calls": []any{纠正调用("one", "apply_patch", 补丁(首个, "one"), 输入.流, 0), 纠正调用("two", "apply_patch", 补丁(秒, "two"), 输入.流, 1)}}, "tool_calls")
					return
				}
				写入文件工具响应(测试器, 当前响应写入器, 输入.流, map[string]any{"content": "done"}, "stop")
			}))
			defer 上游.关闭()
			启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
			消息标识 := 初始化会话模型(测试器, 当前对象, 上游.URL)
			工作区标识 := 结果字符串(测试器, 当前对象.调用(测试器, "workspace.create", map[string]any{"name": "bounded", "root_path": 根}), "id")
			已受理 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "workspace_id": 工作区标识, "stream": 流, "content": 消息内容块集("生成文件")})
			执行标识 := 结果字符串(测试器, 已受理, "run_id")
			当前对象.事件(测试器, "run.completed")
			for 路径, 预期 := range map[string]string{首个: "one\n", 秒: "two\n"} {
				数据, 错误 := 操作系统包.读取文件(路径)
				if 错误 != nil || string(数据) != 预期 {
					测试器.格式化报告致命错误("纠正后的文件缺失: %q %v", 数据, 错误)
				}
			}
			if _, 错误 := 操作系统包.状态信息(从不); !操作系统包.是否不存在(错误) {
				测试器.报告致命错误("坏批次被执行")
			}
			失败, 工具集 := 0, 0
			for _, 步骤 := range 纠正步骤集(测试器, 当前对象, 执行标识) {
				if 步骤.类型 == "tool_call" {
					工具集++
				}
				if 步骤.类型 == "model_call" && 步骤.状态 == 智能体包.失败 {
					失败++
					if 字符串包.包含(纠正数据(测试器, 当前对象, 步骤.标识, "output"), "context_message") {
						测试器.报告致命错误("超限片段进入可恢复模型历史")
					}
				}
			}
			if 失败 != 1 || 工具集 != 2 || 调用集.加载() != 3 {
				测试器.格式化报告致命错误("纠正调用次数异常: failed=%d tools=%d calls=%d", 失败, 工具集, 调用集.加载())
			}
		})
	}
}
```
