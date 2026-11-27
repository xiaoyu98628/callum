```go
package 设置测试

import (
	JSON包 "encoding/json"
	错误判断包 "errors"
	字符串包 "strings"
	测试包 "testing"

	设置包 "github.com/xiaoyu98628/callum/internal/settings"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

func Test执行设置拒绝歧义或者不支持值集(测试器 *测试包.测试器) {
	for _, 原始 := range []string{
		`null`, `[]`, `{}`, `{"approval_policy":"on-request"}`,
		`{"approval_policy":null,"approvals_reviewer":"user"}`,
		`{"approval_policy":"never","approvals_reviewer":"auto_review"}`,
		`{"approval_policy":"on-request","approvals_reviewer":"user","sandbox_mode":"disabled"}`,
		`{"approval_policy":"on-request","approvals_reviewer":"user","sandbox_mode":null}`,
		`{"approval_policy":"on-request","approvals_reviewer":"user","sandbox_mode":""}`,
		`{"approval_policy":"never","approval_policy":"on-request","approvals_reviewer":"user"}`,
		`{"approval_policy":"never","approvals_reviewer":"user"} {}`,
		string([]byte{'{', '"', 0xff, '"', ':', '1', '}'}),
		字符串包.重复(" ", 设置包.最大值字节+1),
	} {
		if _, 错误 := 设置包.解码执行(JSON包.原始JSON消息(原始)); !错误判断包.是否属于(错误, 设置包.无效错误) {
			测试器.格式化报告致命错误("接受了非法配置: %v", 错误)
		}
	}
	for _, 选项 := range []string{"on-request", "always", "never"} {
		值, 错误 := 设置包.解码执行(JSON包.原始JSON消息(`{"approval_policy":"` + 选项 + `","approvals_reviewer":"user"}`))
		if 错误 != nil || string(值.审批策略) != 选项 {
			测试器.格式化报告致命错误("拒绝了有效配置: %+v %v", 值, 错误)
		}
	}
}

func Test沙箱默认不重写历史快照集(测试器 *测试包.测试器) {
	原始 := JSON包.原始JSON消息(`{"approval_policy":"on-request","approvals_reviewer":"user"}`)
	当前, 错误 := 设置包.解码执行(原始)
	if 错误 != nil || 当前.沙箱模式 != 工具包.沙箱工作区写入 {
		测试器.格式化报告致命错误("旧全局配置的新任务默认值错误: %+v %v", 当前, 错误)
	}
	var 旧 设置包.快照
	if 错误 := JSON包.反序列化([]byte(`{"revision":1,"approval_policy":"on-request","approvals_reviewer":"user"}`), &旧); 错误 != nil || !旧.有效() || 旧.沙箱模式 != "" {
		测试器.格式化报告致命错误("历史快照被扩大权限: %+v %v", 旧, 错误)
	}
}
```
