```go
package 应用模块

import (
	SHA256包 "crypto/sha256"
	JSON包 "encoding/json"
	格式化包 "fmt"
	输入输出包 "io"
	HTTP包 "net/http"
	HTTP测试包 "net/http/httptest"
	操作系统包 "os"
	文件路径包 "path/filepath"
	测试包 "testing"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	配置模块包 "github.com/xiaoyu98628/callum/internal/config"
)

func Test会话目录审批通过标准输入输出(测试器 *测试包.测试器) {
	根, 错误 := 文件路径包.求值符号链接集(测试器.创建临时目录())
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	子级 := 文件路径包.合并(根, "child", "note.txt")
	if 错误 := 操作系统包.创建目录全部(文件路径包.目录(子级), 0700); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 操作系统包.写入文件(子级, []byte("directory-permission-content"), 0600); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	外部 := 根 + "-sibling"
	if 错误 := 操作系统包.创建目录(外部, 0700); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	测试器.注册清理函数(func() {
		if 错误 := 操作系统包.移除全部(外部); 错误 != nil {
			测试器.错误文本(错误)
		}
	})
	其他 := 文件路径包.合并(外部, "outside.txt")
	if 错误 := 操作系统包.写入文件(其他, []byte("outside"), 0600); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	链接 := 文件路径包.合并(根, "escape")
	if 错误 := 操作系统包.符号链接(外部, 链接); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	已创建 := 文件路径包.合并(根, "new.txt")
	上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
		var 请求 struct {
			流   bool             `json:"stream"`
			工具集 []JSON包.原始JSON消息 `json:"tools"`
			消息集 []struct {
				角色   string `json:"role"`
				内容   string `json:"content"`
				调用标识 string `json:"tool_call_id"`
			} `json:"messages"`
		}
		if 错误 := JSON包.新建解码器(当前请求.正文).解码(&请求); 错误 != nil {
			测试器.错误文本(错误)
			当前响应写入器.写入头部(400)
			return
		}
		if len(请求.工具集) == 0 {
			写入文件工具响应(测试器, 当前响应写入器, 请求.流, map[string]any{"content": "目录授权"}, "stop")
			return
		}
		文本 := ""
		for _, 映射 := range 请求.消息集 {
			if 映射.角色 == "user" {
				文本 = 映射.内容
			}
		}
		最后 := 请求.消息集[len(请求.消息集)-1]
		名称, 参数, 标识 := "read_file", map[string]any{"path": 子级}, "read-child"
		if 最后.角色 == "tool" {
			写入文件工具响应(测试器, 当前响应写入器, 请求.流, map[string]any{"content": "完成"}, "stop")
			return
		}
		switch 文本 {
		case "sibling":
			参数["path"] = 其他
		case "symlink":
			参数["path"] = 文件路径包.合并(链接, "outside.txt")
		case "write", "write-child", "write-sibling", "write-symlink", "write-other-conversation", "write-revoked":
			路径 := 已创建
			switch 文本 {
			case "write-child":
				路径 = 文件路径包.合并(根, "nested", "created.txt")
			case "write-sibling":
				路径 = 文件路径包.合并(外部, "created.txt")
			case "write-symlink":
				路径 = 文件路径包.合并(链接, "created-via-link.txt")
			case "write-other-conversation":
				路径 = 文件路径包.合并(根, "other-conversation.txt")
			case "write-revoked":
				路径 = 文件路径包.合并(根, "revoked.txt")
			}
			名称, 参数 = "apply_patch", map[string]any{"patch": "*** Begin Patch\n*** Add File: " + 路径 + "\n+created through directory approval\n*** End Patch", "base_versions": []any{}}
		case "command":
			名称, 参数 = "run_command", map[string]any{"command": []string{"echo", "no-execution"}, "cwd": 根, "sandbox_permissions": "require_escalated", "justification": "验证目录授权不能放行沙箱外命令"}
		case "destructive":
			名称, 参数 = "apply_patch", map[string]any{"patch": "*** Begin Patch\n*** Update File: " + 子级 + "\n@@\n-directory-permission-content\n+replacement\n*** End Patch", "base_versions": []any{map[string]string{"path": 子级, "sha256": 格式化包.格式化字符串("%x", SHA256包.校验和256([]byte("directory-permission-content")))}}}
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
		写入文件工具响应(测试器, 当前响应写入器, 请求.流, map[string]any{"tool_calls": []any{调用}}, "tool_calls")
	}))
	defer 上游.关闭()
	启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	消息标识 := 初始化会话模型(测试器, 当前对象, 上游.URL)
	var 会话标识 string
	启动 := func(文本 string, 新建 bool) string {
		参数 := map[string]any{"stream": true, "content": 消息内容块集(文本)}
		if 新建 || 会话标识 == "" {
			参数["model_id"] = 消息标识
		} else {
			参数["conversation_id"] = 会话标识
		}
		帧 := 当前对象.调用(测试器, "conversation.send", 参数)
		if 会话标识 == "" {
			会话标识 = 结果字符串(测试器, 帧, "conversation_id")
		}
		return 结果字符串(测试器, 帧, "run_id")
	}
	决定 := func(范围, 决定 string, 预期选项 bool) 智能体包.审批 {
		var 事件 struct {
			审批 智能体包.审批 `json:"approval"`
		}
		if 错误 := JSON包.反序列化(当前对象.事件(测试器, "approval.requested").参数, &事件); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		if (事件.审批.目录范围 != nil) != 预期选项 {
			测试器.格式化报告致命错误("目录选项不符: %+v", 事件.审批)
		}
		for _, 无效 := range []any{"", "global", nil, 1, map[string]any{"root_path": 根}} {
			帧 := 当前对象.调用(测试器, "approval.decide", map[string]any{"approval_id": 事件.审批.标识, "decision": 决定, "scope": 无效})
			if 帧.错误 == nil || 帧.错误.代码 != -32602 {
				测试器.格式化报告致命错误("接受非法授权范围: %+v", 帧)
			}
		}
		要求成功(测试器, 当前对象.调用(测试器, "approval.decide", map[string]any{"approval_id": 事件.审批.标识, "decision": 决定, "scope": 范围}))
		return 事件.审批
	}
	断言审批集 := func(执行标识 string, 预期 int) {
		帧 := 当前对象.调用(测试器, "approval.list", map[string]any{"run_id": 执行标识})
		要求成功(测试器, 帧)
		var 行集 []智能体包.审批
		if 错误 := JSON包.反序列化(帧.结果, &行集); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		if len(行集) != 预期 {
			测试器.格式化报告致命错误("审批次数不符: want=%d actual=%s", 预期, 帧.结果)
		}
	}
	首个 := 启动("write", false)
	甲 := 决定(智能体包.审批会话目录, 智能体包.审批已批准, true)
	if 甲.目录范围.根路径 != 根 || 甲.目录范围.附件访问器 != "read_write" {
		测试器.报告致命错误("目录范围或影响类别错误")
	}
	当前对象.事件(测试器, "run.completed")
	断言审批集(首个, 1) // 写授权覆盖此会话的实际父目录及后代。
	列表 := 当前对象.调用(测试器, "approval.directory.list", map[string]any{"conversation_id": 会话标识})
	要求成功(测试器, 列表)
	var 授权集 []智能体包.目录授权
	if 错误 := JSON包.反序列化(列表.结果, &授权集); 错误 != nil || len(授权集) != 1 {
		测试器.格式化报告致命错误("未保存目录授权: %s %v", 列表.结果, 错误)
	}
	重复 := 当前对象.调用(测试器, "approval.decide", map[string]any{"approval_id": 甲.标识, "decision": "approved", "scope": 智能体包.审批一次})
	if 重复.错误 == nil || 重复.错误.代码 != 1003 {
		测试器.报告致命错误("已决定的授权范围被更改")
	}
	当前对象.停止(测试器)
	当前对象 = 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	读取 := 启动("read", false)
	当前对象.事件(测试器, "run.completed")
	断言审批集(读取, 0)
	for _, 文本 := range []string{"sibling", "symlink"} {
		运行 := 启动(文本, false)
		当前对象.事件(测试器, "run.completed")
		断言审批集(运行, 0) // 纯读取不再需要额外目录授权。
	}
	for _, 文本 := range []string{"write-sibling", "write-symlink", "command"} {
		运行 := 启动(文本, false)
		决定(智能体包.审批一次, 智能体包.审批已拒绝, 文本 != "command")
		当前对象.事件(测试器, "run.completed")
		断言审批集(运行, 1)
	}
	for _, 路径 := range []string{文件路径包.合并(外部, "created.txt"), 文件路径包.合并(外部, "created-via-link.txt")} {
		if _, 错误 := 操作系统包.状态信息(路径); !操作系统包.是否不存在(错误) {
			测试器.报告致命错误("目录写授权越界")
		}
	}
	其他执行 := 启动("write-other-conversation", true)
	决定(智能体包.审批一次, 智能体包.审批已拒绝, true)
	当前对象.事件(测试器, "run.completed")
	断言审批集(其他执行, 1)
	if _, 错误 := 操作系统包.状态信息(已创建); 错误 != nil {
		测试器.格式化报告致命错误("明确的目录写授权未生效: %v", 错误)
	}
	子级写入 := 启动("write-child", false)
	当前对象.事件(测试器, "run.completed")
	断言审批集(子级写入, 0)
	if _, 错误 := 操作系统包.状态信息(文件路径包.合并(根, "nested", "created.txt")); 错误 != nil {
		测试器.格式化报告致命错误("目录写授权未覆盖子目录: %v", 错误)
	}
	破坏性 := 启动("destructive", false)
	决定(智能体包.审批一次, 智能体包.审批已拒绝, false)
	当前对象.事件(测试器, "run.completed")
	断言审批集(破坏性, 1)
	if 实际, 错误 := 操作系统包.读取文件(子级); 错误 != nil || string(实际) != "directory-permission-content" {
		测试器.格式化报告致命错误("目录读写授权放行了大范围覆盖: %q %v", 实际, 错误)
	}
	for _, 模式 := range []string{"always", "never"} {
		设置审批策略(测试器, 当前对象, 模式)
		运行 := 启动("read", false)
		if 模式 == "always" {
			决定(智能体包.审批一次, 智能体包.审批已拒绝, false)
		}
		当前对象.事件(测试器, "run.completed")
		预期 := 0
		if 模式 == "always" {
			预期 = 1
		}
		断言审批集(运行, 预期)
	}
	设置审批策略(测试器, 当前对象, "on-request")
	帧 := 当前对象.调用(测试器, "approval.directory.list", map[string]any{"conversation_id": 会话标识})
	要求成功(测试器, 帧)
	if 错误 := JSON包.反序列化(帧.结果, &授权集); 错误 != nil || len(授权集) != 1 {
		测试器.格式化报告致命错误("写授权缺失: %s %v", 帧.结果, 错误)
	}
	for _, 组 := range 授权集 {
		要求成功(测试器, 当前对象.调用(测试器, "approval.directory.revoke", map[string]any{"conversation_id": 会话标识, "id": 组.标识}))
	}
	运行 := 启动("write-revoked", false)
	决定(智能体包.审批一次, 智能体包.审批已拒绝, true)
	当前对象.事件(测试器, "run.completed")
	断言审批集(运行, 1)
	读取 = 启动("read", false)
	当前对象.事件(测试器, "run.completed")
	断言审批集(读取, 0) // 撤销额外写授权不改变默认只读策略。
	当前对象.停止(测试器)
}
```
