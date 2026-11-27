```go
package 会话

import (
	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	设置包 "github.com/xiaoyu98628/callum/internal/settings"
)

// 上下文用量请求 只读预览当前有效上下文，不创建消息、调用模型或触发压缩。
type 上下文用量请求 struct {
	会话标识  string  `json:"conversation_id,omitempty"`
	工作区标识 *string `json:"workspace_id,omitempty"`
	模型标识  string  `json:"model_id,omitempty"`
	草稿图片集 int     `json:"draft_images,omitempty"`
	草稿    string  `json:"draft,omitempty"`
}

// 上下文用量 的占用来自服务端估算；实际输入用量独立保存，不以累计计费用量代替。
// 修订号 仅在当前 运行时 内递增；客户端重连后必须重新查询。
type 上下文用量 struct {
	会话标识   string `json:"conversation_id"`
	模型标识   string `json:"model_id"`
	执行标识   string `json:"run_id,omitempty"`
	修订号    int64  `json:"revision"`
	更新时间   int64  `json:"updated_at"`
	状态     string `json:"state"`
	原因     string `json:"reason,omitempty"`
	估算词元   *int64 `json:"estimated_tokens"`
	草稿词元   int64  `json:"draft_tokens"`
	输入上限   *int64 `json:"input_limit"`
	上下文窗口  *int64 `json:"context_window"`
	输出预留   int64  `json:"output_reserve"`
	安全余量   int64  `json:"safety_margin"`
	估算方法   string `json:"estimation_method"`
	实际输入词元 *int64 `json:"actual_input_tokens"`
}

// 上下文状态 固定只读历史的边界；大段正文由 上下文页 分批读取。
type 上下文状态 struct {
	会话       会话
	设置       设置包.快照
	上下文      *上下文版本
	通过序号     int64
	最后用户执行标识 string
	活动执行     *智能体包.执行
	实际输入词元   *int64
	实际模型标识   string
}
```
