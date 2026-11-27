```go
package 推理调用

import (
	错误判断包 "errors"
	字符串包 "strings"
	测试包 "testing"

	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

func Test公开事件丢弃错误原因(测试器 *测试包.测试器) {
	原因 := 错误判断包.新建("secret-key-" + 字符串包.重复("body", 1024))
	事件 := 公开事件(事件{类型: "failed", 执行标识: "r", 错误: 模型包.新建调用错误("request", 401, "request", 原因)})
	值, 成功 := 事件.数据.(通知)
	if !成功 || 值.失败 == nil || 值.失败.HTTP状态 != 401 || 值.失败.供应商请求标识 != "request" || 值.失败.消息 != "模型调用失败" {
		测试器.格式化报告致命错误("受控错误丢失: %+v", 值)
	}
	if 字符串包.包含(值.失败.消息, "secret") {
		测试器.报告致命错误("错误正文进入公共事件")
	}
}
```
