package service

import (
	"context"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/executor"
	"github.com/xiaoyu98628/callum/internal/conversation"
)

// ExecutionCoordinator 是会话受理实际需要的执行协调能力，不包含工具、审批及步骤写入实现。
// 具体执行器由装配层注入；会话服务仍负责自己的持久化操作与提交后通知。
type ExecutionCoordinator interface {
	PrepareExecution(*executor.Lease, conversation.Execution, agent.Resolved) func(context.Context)
	Abandon(*executor.Lease, conversation.Execution, error)
	ExecuteCompaction(context.Context, *executor.Lease, conversation.Execution, agent.Resolved)
	SteeringLease(string) *executor.Lease
	AcceptSteering(*executor.Lease, func() (conversation.FollowUp, error)) error
	Get(context.Context, string) (agent.Run, error)
	LiveContextUsage(conversation.ContextUsageRequest) (conversation.ContextUsage, bool, error)
	PreviewContext(context.Context, conversation.ContextUsageRequest, conversation.ContextState, agent.Resolved) (conversation.ContextUsage, error)
}
