```go
package 会话

import (
	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

// 会话 是会话展示信息，首次发送时与消息、执行一起创建。
type 会话 struct {
	标识    string  `json:"id"`
	工作区标识 *string `json:"workspace_id"`
	标题    string  `json:"title"`
	标题来源  string  `json:"title_source"`
	模型    模型包.快照  `json:"model"`
	置顶位于  *int64  `json:"pinned_at"`
	已归档位于 *int64  `json:"archived_at"`
	删除时间  *int64  `json:"deleted_at"`
	创建时间  int64   `json:"created_at"`
	更新时间  int64   `json:"updated_at"`
}

// 消息 仅保存用户消息及回答，工具、审批、补充参数保存在执行步骤。
type 消息 struct {
	标识   string `json:"id"`
	会话标识 string `json:"conversation_id"`
	执行标识 string `json:"run_id"`
	序号   int64  `json:"sequence"`
	// 引导顺序 为同一执行中的引导排序。引导之后步骤 锚定调用过程中的展示位置。
	引导顺序   int64     `json:"steering_order,omitempty"`
	引导之后步骤 *int64    `json:"steering_after_step,omitempty"`
	角色     string    `json:"role"`
	内容     []模型包.内容块 `json:"content"`
	状态     string    `json:"status"`
	创建时间   int64     `json:"created_at"`
	更新时间   int64     `json:"updated_at"`
	已完成位于  *int64    `json:"completed_at"`
}

// 历史页 按当前分支的执行分页，消息与独立压缩共享同一游标，避免跨页错配。
// 之前执行标识 是下一页的排他上界；空页不推进游标。
type 历史页 struct {
	消息集    []消息      `json:"messages"`
	执行集    []智能体包.执行 `json:"runs"`
	之前执行标识 string    `json:"before_run_id"`
	还有更多   bool      `json:"has_more"`
}

// 更新 是业务更新输入；模型只接受本地标识，快照由服务端解析。
type 更新 struct {
	标题      *string `json:"title,omitempty"`
	模型标识    string  `json:"model_id,omitempty"`
	工作区标识   *string `json:"workspace_id,omitempty"`
	解除关联工作区 bool    `json:"detach_workspace,omitempty"`
	置顶      *bool   `json:"pinned,omitempty"`
	已归档     *bool   `json:"archived,omitempty"`
	已删除     *bool   `json:"-"`
}

// 变更 是服务校验后的仓储写入参数；模型 仅使用服务端解析的快照。
type 变更 struct {
	标题      *string `json:"title,omitempty"`
	模型      *模型包.快照 `json:"-"`
	工作区标识   *string `json:"workspace_id,omitempty"`
	解除关联工作区 bool    `json:"detach_workspace,omitempty"`
	置顶      *bool   `json:"pinned,omitempty"`
	已归档     *bool   `json:"archived,omitempty"`
	已删除     *bool   `json:"-"`
}

type 筛选 struct {
	工作区标识 *string `json:"workspace_id,omitempty"`
	未绑定   bool    `json:"unbound,omitempty"`
	已归档   bool    `json:"archived"`
	已删除   bool    `json:"deleted"`
	上限    int     `json:"limit"`
	偏移    int     `json:"offset"`
}

// 开始 是一次发送、编辑重发或继续任务的原子写入输入，所有 标识 由业务层生成。
type 开始 struct {
	后续消息     *后续消息开始
	创建       bool
	会话标识     string
	工作区标识    *string
	用户消息标识   string
	助手消息标识   string
	执行标识     string
	步骤标识     string
	编辑重发消息标识 string
	编辑重发执行标识 string
	继续       bool
	继续图片输入   bool
	模型       模型包.快照
	协议       string
	内容       []模型包.内容块
	流        bool
}

// 执行 是提交后的执行快照，不持有数据库连接或事务。
type 执行 struct {
	// 历史更多 表示受理只装配有界前缀，执行阶段继续压缩并读取余下历史。
	历史更多     bool
	上下文      *上下文版本
	已移除消息标识集 []string
	已继续      bool
	已发布交付物集  []模型包.内容块
	已创建      bool
	会话       会话
	用户       消息
	助手       消息
	执行       智能体包.执行
	步骤       智能体包.步骤
	历史       []消息
}

// 发送 是发送或编辑重发的输入。首次提问不提供 会话标识；模型标识 可在已有会话省略。
type 发送 struct {
	会话标识   string    `json:"conversation_id,omitempty"`
	工作区标识  *string   `json:"workspace_id,omitempty"`
	模型标识   string    `json:"model_id,omitempty"`
	消息标识   string    `json:"message_id,omitempty"`
	预期执行标识 string    `json:"expected_run_id,omitempty"`
	已保留附件集 []int     `json:"retained_attachments,omitempty"`
	内容     []模型包.内容块 `json:"content"`
	流      *bool     `json:"stream,omitempty"`
}

// 继续 标识用户要继续的最新一轮；服务端在受理事务中复核所属会话和执行状态。
type 继续 struct {
	会话标识 string `json:"conversation_id"`
	执行标识 string `json:"run_id"`
	模型标识 string `json:"model_id,omitempty"`
}
```
