```go
package 会话

import (
	上下文包 "context"
	错误判断包 "errors"
	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	测试包 "testing"
)

func Test附件错误判断保留可操作(测试器 *测试包.测试器) {
	for _, 错误 := range []error{模型包.附件错误, 错误判断包.合并(模型包.附件错误, 错误判断包.新建("private-path"))} {
		当前条目 := 公开错误(错误)
		数据, 成功 := 当前条目.数据.(map[string]any)
		if 当前条目.代码 != 1013 || 当前条目.消息 != 模型包.附件错误.错误文本() || !成功 || 数据["kind"] != "attachment_unavailable" {
			测试器.格式化报告致命错误("附件错误未保留受控分类: %+v", 当前条目)
		}
	}
	当前条目 := 公开错误(错误判断包.合并(智能体包.存储错误, 模型包.附件错误, 上下文包.已取消错误))
	if 当前条目.代码 != -32603 {
		测试器.格式化报告致命错误("持久化失败被附件分类掩盖: %+v", 当前条目)
	}
}
```
