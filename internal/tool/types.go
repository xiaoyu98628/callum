// Package tool 定义工具执行契约，权限和工作区来自服务端，不接受模型覆盖。
package tool

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/xiaoyu98628/callum/internal/model"
)

const MaxResultBytes = 64 << 10
const MaxArgumentBytes = 16 << 10

var (
	ErrInvalid     = errors.New("工具参数无效")
	ErrDenied      = errors.New("操作超出授权工作区或权限范围")
	ErrUnavailable = errors.New("工具不可用")
	ErrLimit       = errors.New("工具输入或结果超过限制")
)

// Effect 描述工具可能产生的操作影响，由可信工具实现声明。
type Effect string

const (
	EffectRead        Effect = "read"        // 工作区内读取，不修改文件。
	EffectWrite       Effect = "write"       // 工作区内新建或普通修改，不包含大范围覆盖。
	EffectDestructive Effect = "destructive" // 删除、大范围覆盖等破坏性操作。
	EffectExternal    Effect = "external"    // 向外部发送数据。
	EffectUnknown     Effect = "unknown"     // 尚不能明确判断操作影响。
)

// Valid 判断操作类别是否受支持；缺少类别时由审批策略按未知操作处理。
func (e Effect) Valid() bool {
	switch e {
	case EffectRead, EffectWrite, EffectDestructive, EffectExternal, EffectUnknown:
		return true
	default:
		return false
	}
}

// Definition 由可信注册代码提供；Effects 不能由模型或调用参数声明。
// 一个工具可包含多个影响，不能用读取类别抵消删除或外发影响。
type Definition struct {
	model.ToolDefinition
	Source  string   `json:"source"`
	Version string   `json:"version"`
	Effects []Effect `json:"effects"`
}

// Scope 固定本次调用的工作区授权范围。
type Scope struct {
	ID       string `json:"id"`
	RootPath string `json:"root_path"`
}

// Tool 的 Validate 返回规范化参数；Execute 只能访问传入的工作区根句柄。
// 根句柄及其生命周期由执行器持有，具体工具不得关闭根句柄。
type Tool interface {
	Definition() Definition
	Validate(json.RawMessage) (json.RawMessage, error)
	Execute(context.Context, *os.Root, json.RawMessage) (json.RawMessage, error)
}
