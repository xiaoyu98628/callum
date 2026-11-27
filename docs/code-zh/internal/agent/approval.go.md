```go
package 智能体

import (
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

const 等待中审批 = "awaiting_approval"
const 审批待处理 = "pending"
const 审批已批准 = "approved"
const 审批已拒绝 = "rejected"
const 审批已取消 = "cancelled"
const 审批自动审核方 = "auto_review"

var 冲突错误 = 错误判断包.新建("执行或审批状态冲突")
var 审批无效错误 = 错误判断包.新建("审批参数无效")

// 审批 是一个确定工具操作的审批记录，请求 由服务端生成，不接受客户端替换。
type 审批 struct {
	标识    string         `json:"id"`
	执行标识  string         `json:"run_id"`
	步骤标识  string         `json:"step_id"`
	请求    JSON包.原始JSON消息 `json:"request"`
	请求哈希  string         `json:"request_hash"`
	状态    string         `json:"status"`
	已决定依据 *string        `json:"decided_by"`
	审核    *审批审核          `json:"review,omitempty"`
	决定说明  *string        `json:"decision_note"`
	创建时间  int64          `json:"created_at"`
	更新时间  int64          `json:"updated_at"`
	已决定位于 *int64         `json:"decided_at"`
	目录范围  *工具包.目录范围      `json:"directory_scope,omitempty"`
	决定范围  string         `json:"decision_scope,omitempty"`
	目录授权  *目录授权          `json:"directory_grant,omitempty"`
}

// 审批审核 关联审核模型步骤。转人工时仍保留原始审核结论。
type 审批审核 struct {
	步骤标识 string `json:"step_id"`
	决定   string `json:"decision"`
	原因   string `json:"reason"`
}

type 审批筛选 struct {
	执行标识 string `json:"run_id,omitempty"`
	状态   string `json:"status,omitempty"`
	上限   int    `json:"limit,omitempty"`
	偏移   int    `json:"offset,omitempty"`
}

// 审批仓储 的创建与决定均须校验执行和步骤状态，并以事务保存。
type 审批仓储 interface {
	创建审批(上下文包.上下文, 审批) (审批, error)
	获取审批(上下文包.上下文, string) (审批, error)
	列表审批集(上下文包.上下文, 审批筛选) ([]审批, error)
	决定审批(上下文包.上下文, string, string, *string, *string, ...string) (审批, error)
	取消审批(上下文包.上下文, string) (审批, error)
	列表目录授权集(上下文包.上下文, string, int, int) ([]目录授权, error)
	撤销目录授权(上下文包.上下文, string, string) (目录授权, error)
}

const 审批一次 = "once"
const 审批会话目录 = "conversation_directory"

// 目录授权 保留会话内目录授权的来源和撤销事实，不复制为后续操作的用户审批。
type 目录授权 struct {
	标识   string   `json:"id"`
	会话标识 string   `json:"conversation_id"`
	审批标识 string   `json:"approval_id"`
	范围   工具包.目录范围 `json:"scope"`
	创建时间 int64    `json:"created_at"`
	撤销时间 *int64   `json:"revoked_at"`
}
```
