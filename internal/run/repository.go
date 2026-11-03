package run

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
}
