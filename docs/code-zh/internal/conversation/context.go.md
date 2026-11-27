```go
package 会话

import (
	上下文包 "context"
	JSON包 "encoding/json"
	切片包 "slices"
)

// 上下文来源 固定原始消息和对应工具轨迹；指纹不包含凭据。
type 上下文来源 struct {
	消息标识 string `json:"message_id"`
	执行标识 string `json:"run_id"`
	序号   int64  `json:"sequence"`
	指纹   string `json:"fingerprint"`
}

// 上下文版本 是可追溯的历史摘要。原文保留，只有 活动 版本参与模型请求。
type 上下文版本 struct {
	标识        string         `json:"id"`
	会话标识      string         `json:"conversation_id"`
	版本        int64          `json:"version"`
	基准版本标识    *string        `json:"base_version_id"`
	已生成依据步骤标识 string         `json:"generated_by_step_id"`
	通过序号      int64          `json:"through_sequence"`
	摘要        string         `json:"summary,omitempty"`
	来源集       []上下文来源        `json:"source_manifest,omitempty"`
	统计        JSON包.原始JSON消息 `json:"size_stats,omitempty"`
	状态        string         `json:"status"`
	创建时间      int64          `json:"created_at"`
	更新时间      int64          `json:"updated_at"`
	已激活位于     *int64         `json:"activated_at"`
	已失效位于     *int64         `json:"invalidated_at"`
}

// 压缩 不创建用户消息；模型标识 省略时使用会话当前模型。
type 压缩 struct {
	会话标识 string `json:"conversation_id"`
	模型标识 string `json:"model_id,omitempty"`
}

// 上下文仓储 管理摘要生命周期；生成模型输出期间不持有事务。
type 上下文仓储 interface {
	上下文状态(上下文包.上下文, string) (上下文状态, error)
	上下文页(上下文包.上下文, string, int64, int64) ([]消息, bool, error)
	加载上下文历史(上下文包.上下文, string, int64) ([]消息, *上下文版本, bool, error)
	活动上下文(上下文包.上下文, string) (*上下文版本, error)
	上下文版本(上下文包.上下文, string, string) (上下文版本, error)
	上下文版本集(上下文包.上下文, string, int, int) ([]上下文版本, error)
	开始压缩(上下文包.上下文, 开始) (执行, error)
	创建上下文(上下文包.上下文, 上下文版本) (上下文版本, error)
	激活上下文(上下文包.上下文, 上下文版本) error
	失败上下文(上下文包.上下文, string) error
}

// 复制 隔离受理回调借用的版本数据，防止修改真实执行的摘要和来源。
func (当前上下文版本 上下文版本) 复制() 上下文版本 {
	当前上下文版本.来源集 = 切片包.复制(当前上下文版本.来源集)
	当前上下文版本.统计 = 切片包.复制(当前上下文版本.统计)
	if 当前上下文版本.基准版本标识 != nil {
		当前上下文版本.基准版本标识 = new(*当前上下文版本.基准版本标识)
	}
	if 当前上下文版本.已激活位于 != nil {
		当前上下文版本.已激活位于 = new(*当前上下文版本.已激活位于)
	}
	if 当前上下文版本.已失效位于 != nil {
		当前上下文版本.已失效位于 = new(*当前上下文版本.已失效位于)
	}
	return 当前上下文版本
}
```
