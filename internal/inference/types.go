package inference

import (
	"context"

	"github.com/xiaoyu98628/callum/internal/model"
)

// MaxTextBytes 限制单次调用累计的文本字节数，不代表模型的 Token 容量。
const MaxTextBytes = 1 << 20

// Resolver 为一次调用取得配置快照和统一客户端，不得在返回后持有数据库事务。
type Resolver func(context.Context, string) (model.Client, string, error)

// Request 由调用方提供完整文本上下文，不负责保存会话历史。
type Request struct {
	ModelID  string
	Stream   bool
	Messages []model.Message
}

// Event 是执行事件，Err 仅供通信边界转换，不能直接序列化错误链。
type Event struct {
	Type              string
	RunID             string
	Text              *string
	ProviderRequestID string
	Err               error
}
