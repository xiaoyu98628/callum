package agent

import (
	"context"

	"github.com/xiaoyu98628/callum/internal/model"
)

// Repository 保存执行及步骤；恢复只能在取得应用数据目录独占运行权后调用。
type Repository interface {
	Get(context.Context, string) (Run, error)
	List(context.Context, string, int, int) ([]Run, error)
	GetStep(context.Context, string) (Step, error)
	Steps(context.Context, string, int64, int) ([]Step, error)
	CreateStep(context.Context, Step) (Step, error)
	SaveStep(context.Context, Step) error
	SaveMessage(context.Context, string, []model.Block, string) error
	Finish(context.Context, Run) error
	Recover(context.Context) error
	// StartTool 原子认领尚未执行的工具步骤，核验审批及当前执行仍有效。
	StartTool(context.Context, string, string, string, string) (Step, error)
	Resume(context.Context, string) error
	// Transcript 读取已完成执行的模型、工具上下文，不包含辅助标题或重发旧分支。
	Transcript(context.Context, string) (map[string][]model.Message, error)
}
