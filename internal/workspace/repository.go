package workspace

import "context"

// Repository 实现工作区持久化；调用返回前释放连接，变更与执行检查必须原子完成。
type Repository interface {
	Create(context.Context, Workspace) (Workspace, error)
	Get(context.Context, string) (Workspace, error)
	List(context.Context, Filter) ([]Workspace, error)
	Update(context.Context, string, Change) (Workspace, error)
}
