```go
package 智能体

import (
	上下文包 "context"
	JSON包 "encoding/json"

	模型包 "github.com/xiaoyu98628/callum/internal/model"
	设置包 "github.com/xiaoyu98628/callum/internal/settings"
)

const (
	待处理 = "pending"
	运行中 = "running"
	已完成 = "completed"
	失败  = "failed"
	已取消 = "cancelled"
	已中断 = "interrupted"
)

// 执行 保存一次持久化执行的状态，资源由执行调度器管理。
type 执行 struct {
	标识     string  `json:"id"`
	会话标识   string  `json:"conversation_id"`
	替换执行标识 *string `json:"replaces_run_id"`
	模型     模型包.快照  `json:"model"`
	流      bool    `json:"stream"`
	设置快照   *设置包.快照 `json:"settings_snapshot"`
	状态     string  `json:"status"`
	错误类别   *string `json:"error_kind"`
	错误消息   *string `json:"error_message"`
	创建时间   int64   `json:"created_at"`
	已继续位于  *int64  `json:"resumed_at,omitempty"`
	更新时间   int64   `json:"updated_at"`
	结束时间   *int64  `json:"finished_at"`
}

// 步骤 保存一次实际操作；模型响应和 用量 使用 JSON 保留协议字段。
type 步骤 struct {
	标识      string         `json:"id"`
	会话标识    string         `json:"conversation_id"`
	执行标识    string         `json:"run_id"`
	父级步骤标识  *string        `json:"parent_step_id"`
	序号      int64          `json:"sequence"`
	类型      string         `json:"type"`
	名称      string         `json:"name"`
	定义      JSON包.原始JSON消息 `json:"definition"`
	状态      string         `json:"status"`
	输入      JSON包.原始JSON消息 `json:"input"`
	输出      JSON包.原始JSON消息 `json:"output"`
	用量      JSON包.原始JSON消息 `json:"usage"`
	模型      *模型包.快照        `json:"model"`
	供应商请求标识 *string        `json:"provider_request_id"`
	工具调用标识  *string        `json:"tool_call_id"`
	错误类别    *string        `json:"error_kind"`
	错误消息    *string        `json:"error_message"`
	创建时间    int64          `json:"created_at"`
	开始时间    *int64         `json:"started_at"`
	更新时间    int64          `json:"updated_at"`
	结束时间    *int64         `json:"finished_at"`
}

// 步骤数据 是指定步骤字段的有界 UTF-8 片段，位置和大小均以字节计算。
type 步骤数据 struct {
	存在    bool
	总计字节  int
	下一个偏移 int
	片段    string
}

// 事件 是与通信协议无关的业务通知，数据 及其引用内容在发布后只读。
type 事件 struct {
	类型       string
	会话标识     string
	执行标识     string
	步骤标识     string
	消息标识     string
	数据       any
	已移除消息标识集 []string
	状态已持久化   *bool
}

// 已解析 是单次执行的配置快照，客户端 的 HTTP 资源仍由装配层管理。
type 已解析 struct {
	客户端    模型包.客户端
	快照     模型包.快照
	协议     string
	图片输入   bool
	上下文窗口  int64
	最大输出词元 int64
}

type 解析器 func(上下文包.上下文, string) (已解析, error)
```
