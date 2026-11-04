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
	ErrInvalid        = errors.New("工具参数无效")
	ErrDenied         = errors.New("操作超出授权工作区或权限范围")
	ErrUnavailable    = errors.New("工具不可用")
	ErrLimit          = errors.New("工具输入或结果超过限制")
	ErrPathBase       = errors.New("相对路径需要工作区，请提供绝对路径或绑定工作区")
	ErrUnknownOutcome = errors.New("操作结果未知，需要核实后再重试")
	ErrPartialOutcome = errors.New("操作仅部分生效，需要核实已完成的部分")
	ErrConflict       = errors.New("文件版本、路径或补丁上下文发生冲突，请重新读取后修改")
)

// Outcome 描述实际副作用，与工具调用是否成功、执行是否取消分别记录。
// 只能由可信工具实现报告；未报告或无法确认时不能推断为未生效。
type Outcome string

const (
	OutcomeNone       Outcome = "none"              // 操作没有修改或外发等副作用。
	OutcomeNotApplied Outcome = "not_applied"       // 确认尚未产生副作用。
	OutcomeApplied    Outcome = "applied"           // 确认请求的操作已经生效。
	OutcomePartial    Outcome = "partially_applied" // 确认只有部分操作生效。
	OutcomeUnknown    Outcome = "unknown"           // 无法确认操作是否或完全生效。
)

// Valid 判断是否为当前契约可表达的事实。
func (o Outcome) Valid() bool {
	return o == OutcomeNone || o == OutcomeNotApplied || o == OutcomeApplied || o == OutcomePartial || o == OutcomeUnknown
}

// MayHaveEffects 用于保留需要后续知晓的操作事实，不表示执行成功或得到授权。
func (o Outcome) MayHaveEffects() bool {
	return o == OutcomeApplied || o == OutcomePartial || o == OutcomeUnknown
}

// Result 保留有界 JSON 输出及独立的副作用事实；返回错误也不得丢弃已经发生的操作。
// Output 在调用返回后归调用方持有，工具不得继续修改；错误时可以没有可用输出。
type Result struct {
	Output  json.RawMessage
	Outcome Outcome
}

// OperationVersion 标识步骤 JSON 中操作事实的兼容版本，不是数据库结构版本。
const OperationVersion = 1

// OutcomeRecord 是步骤和工具上下文中的服务端事实，不能替代审批或权限配置。
// 版本缺失的旧记录保持原义；Recovered 表示从未完成的新步骤确认结果尚不可得。
type OutcomeRecord struct {
	Version     int     `json:"version"`
	Outcome     Outcome `json:"outcome"`
	RequestHash string  `json:"request_hash,omitempty"`
	Recovered   bool    `json:"recovered,omitempty"`
}

// Valid 同时检查契约版本和事实枚举，不推断旧记录的含义。
func (r OutcomeRecord) Valid() bool { return r.Version == OperationVersion && r.Outcome.Valid() }

// UnconfirmedOutcome 只根据可信定义识别无副作用工具；其他已启动操作一律保持未知。
func UnconfirmedOutcome(effects []Effect) Outcome {
	if len(effects) == 0 {
		return OutcomeUnknown
	}
	for _, effect := range effects {
		if effect != EffectRead {
			return OutcomeUnknown
		}
	}
	return OutcomeNone
}

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
	Action          string `json:"action,omitempty"`
	ExpectedSHA256  string `json:"expected_sha256,omitempty"`
	RecoveryPath    string `json:"recovery_path,omitempty"`
	StagingPath     string `json:"staging_path,omitempty"`
}

// Plan 固定多目标操作及额外影响。由可信调用实现生成，不能由模型声明权限。
// AdditionalEffects 只能增加影响，不能抵消工具定义中已有的影响。
type Plan struct {
	Targets           []Target `json:"targets"`
	AdditionalEffects []Effect `json:"additional_effects,omitempty"`
}

// PlannedInvocation 为实际需要多目标或动态影响判定的工具提供审批计划。
// Plan 必须只返回元数据，在 Prepare、审批等待及执行开始前保持不变。
type PlannedInvocation interface {
	Invocation
	Plan() Plan
}

// Invocation 持有一次调用的固定资源；Execute 在使用前重新校验目标。
// 非文件工具的 Target 返回 nil。Close 必须可重复调用；调用创建者负责关闭，
// 包括审批拒绝、取消以及 Prepare 同时返回调用对象和错误时。
type Invocation interface {
	Target() *Target
	Execute(context.Context) (Result, error)
	Close() error
}

// Tool 的 Validate 返回规范化参数；Prepare 固定目标，但不得读取正文或执行操作。
// 工具按自身需求使用工作区，不要求所有工具依赖文件系统或工作区。
type Tool interface {
	Definition() Definition
	Validate(json.RawMessage) (json.RawMessage, error)
	Prepare(context.Context, Scope, json.RawMessage) (Invocation, error)
}
