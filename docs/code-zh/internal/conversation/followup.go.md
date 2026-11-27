```go
package 会话

import (
	错误判断包 "errors"

	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

const (
	后续消息已排队 = "queued"
	后续消息草稿  = "draft"
	后续消息引导  = "steering"
	后续消息已生效 = "applied"
	后续消息已发送 = "sent"
	后续消息已移除 = "removed"
	后续消息上限  = 32
)

var (
	后续消息变化通知错误 = 错误判断包.新建("后续消息或当前执行已变化，请刷新后重试")
	后续消息已发送错误  = 错误判断包.新建("这条消息已开始发送，请在会话中查看")
	后续消息草稿错误   = 错误判断包.新建("请先处理输入框里已取出的消息")
	后续消息上限错误   = 错误判断包.新建("后续消息已达到存储或数量上限")
	已引导错误      = 错误判断包.新建("已收到新的引导，原工具调用未执行")
)

type 后续消息 struct {
	标识      string    `json:"id"`
	会话标识    string    `json:"conversation_id"`
	预期执行标识  string    `json:"expected_run_id"`
	模型标识    string    `json:"model_id"`
	状态      string    `json:"status"`
	内容      []模型包.内容块 `json:"content"`
	位置      int64     `json:"position"`
	修订号     int64     `json:"revision"`
	已生效步骤标识 *string   `json:"applied_step_id,omitempty"`
	回执      *消息       `json:"-"`
	创建时间    int64     `json:"created_at"`
}

type 后续消息输入 struct {
	会话标识   string    `json:"conversation_id"`
	预期执行标识 string    `json:"expected_run_id"`
	模型标识   string    `json:"model_id,omitempty"`
	模式     string    `json:"mode"`
	内容     []模型包.内容块 `json:"content"`
}

type 后续消息变更 struct {
	会话标识   string  `json:"conversation_id"`
	标识     string  `json:"id"`
	修订号    int64   `json:"revision"`
	动作     string  `json:"action"`
	预期执行标识 string  `json:"expected_run_id,omitempty"`
	文本     *string `json:"text,omitempty"`
	模型标识   string  `json:"model_id,omitempty"`
}

type 后续消息开始 struct {
	会话标识   string  `json:"conversation_id"`
	标识     string  `json:"id"`
	修订号    int64   `json:"revision"`
	之后执行标识 string  `json:"after_run_id,omitempty"`
	文本     *string `json:"text,omitempty"`
	模型标识   string  `json:"model_id,omitempty"`
}
```
