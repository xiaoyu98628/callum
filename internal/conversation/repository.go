package conversation

import (
	"context"
	"github.com/xiaoyu98628/callum/internal/model"
)

// Repository 按业务动作保证跨会话、消息、执行和步骤的事务一致性。
// Begin 返回后不得占用连接；生成标题只允许条件更新未被用户修改的默认标题。
type Repository interface {
	ContextRepository
	Get(context.Context, string) (Conversation, error)
	List(context.Context, Filter) ([]Conversation, error)
	Update(context.Context, string, Change) (Conversation, error)
	Message(context.Context, string, string) (Message, error)
	Attachment(context.Context, string, string) (model.Block, error)
	Messages(context.Context, string, int64, int) ([]Message, error)
	History(context.Context, string, string, int) (HistoryPage, error)
	Begin(context.Context, Begin) (Execution, error)
	ApplyTitle(context.Context, string, string, string, string) (Conversation, bool, error)
}
