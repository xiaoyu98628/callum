// Package tool 定义工具执行契约，权限和可选工作区来自服务端，不接受模型覆盖。
package tool

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/xiaoyu98628/callum/internal/model"
)

const MaxResultBytes = 64 << 10
const MaxArgumentBytes = 16 << 10

var (
	ErrInvalid     = errors.New("工具参数无效")
	ErrDenied      = errors.New("操作超出授权工作区或权限范围")
	ErrUnavailable = errors.New("工具不可用")
	ErrLimit       = errors.New("工具输入或结果超过限制")
	ErrPathBase    = errors.New("相对路径需要工作区，请提供绝对路径或绑定工作区")
)

// Effect 描述工具可能产生的操作影响，由可信工具实现声明。
type Effect string

const (
	EffectRead        Effect = "read"        // 读取，不修改资源；文件访问范围另行校验。
	EffectWrite       Effect = "write"       // 新建或普通修改，不包含大范围覆盖。
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

// Scope 提供可选的工作区路径基准；零值表示会话未绑定工作区。
type Scope struct {
	ID       string `json:"id"`
	RootPath string `json:"root_path"`
}

// Target 记录文件工具固定的实际目标，目录操作的范围限于该目录。
// WithinWorkspace 由可信实现依据实际路径判断，不能由模型参数指定。
type Target struct {
	RequestedPath   string `json:"requested_path"`
	Path            string `json:"path"`
	Kind            string `json:"kind"`
	WithinWorkspace bool   `json:"within_workspace"`
}

// Invocation 持有一次调用的固定资源；Execute 在使用前重新校验目标。
// 非文件工具的 Target 返回 nil。Close 必须可重复调用；调用创建者负责关闭，
// 包括审批拒绝、取消以及 Prepare 同时返回调用对象和错误时。
type Invocation interface {
	Target() *Target
	Execute(context.Context) (json.RawMessage, error)
	Close() error
}

// Tool 的 Validate 返回规范化参数；Prepare 固定目标，但不得读取正文或执行操作。
// 工具按自身需求使用工作区，不要求所有工具依赖文件系统或工作区。
type Tool interface {
	Definition() Definition
	Validate(json.RawMessage) (json.RawMessage, error)
	Prepare(context.Context, Scope, json.RawMessage) (Invocation, error)
}
