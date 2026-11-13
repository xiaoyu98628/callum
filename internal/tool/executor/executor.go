package executor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/registry"
)

// Operation 是服务端生成的审批快照，与实际执行使用相同的规范化参数。
type Operation struct {
	Tool      tool.Definition `json:"tool"`
	Workspace *tool.Scope     `json:"workspace"`
	Target    *tool.Target    `json:"target,omitempty"`
	Plan      *tool.Plan      `json:"plan,omitempty"`
	Details   json.RawMessage `json:"details,omitempty"`
	Arguments json.RawMessage `json:"arguments"`
	Summary   string          `json:"summary"`
}

// Prepared 持有固定操作和调用资源，创建者必须 Close；等待审批期间不能替换操作。
type Prepared struct {
	invocation tool.Invocation
	target     *tool.Target
	instance   tool.Tool
	definition tool.Definition
	plan       *tool.Plan
	details    json.RawMessage
	effects    []tool.Effect
	request    json.RawMessage
	hash       string
	closeOnce  sync.Once
	closeErr   error
	boundary   *tool.CommandBoundary
}

func (p *Prepared) Request() json.RawMessage { return slices.Clone(p.request) }
func (p *Prepared) Hash() string             { return p.hash }
func (p *Prepared) CommandBoundary() *tool.CommandBoundary {
	if p.boundary == nil {
		return nil
	}
	return new(*p.boundary)
}
func (p *Prepared) Definition() tool.Definition {
	d := p.definition
	d.Parameters = slices.Clone(d.Parameters)
	d.Effects = slices.Clone(p.effects)
	return d
}
func (p *Prepared) OutsideWorkspace() bool {
	if p.target != nil && !p.target.WithinWorkspace {
		return true
	}
	if p.plan != nil {
		for _, target := range p.plan.Targets {
			if !target.WithinWorkspace {
				return true
			}
		}
	}
	return false
}
func (p *Prepared) Close() error {
	p.closeOnce.Do(func() { p.closeErr = p.invocation.Close() })
	return p.closeErr
}

// Executor 在审批前校验参数并固定调用，具体资源由工具持有，不强制要求工作区。
type Executor struct {
	registry *registry.Registry
	timeout  time.Duration
}

func New(r *registry.Registry, timeout time.Duration) (*Executor, error) {
	if r == nil || timeout <= 0 {
		return nil, tool.ErrInvalid
	}
	return &Executor{registry: r, timeout: timeout}, nil
}

func (e *Executor) Definitions() []tool.Definition { return e.registry.Definitions() }

func (e *Executor) Prepare(ctx context.Context, scope tool.Scope, name string, args json.RawMessage) (*Prepared, error) {
	return e.PrepareWithPermissions(ctx, scope, name, args, tool.ExecutionPermissions{})
}

func (e *Executor) PrepareWithPermissions(ctx context.Context, scope tool.Scope, name string, args json.RawMessage, permissions tool.ExecutionPermissions) (*Prepared, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if scope != (tool.Scope{}) && (scope.ID == "" || !filepath.IsAbs(scope.RootPath)) {
		return nil, tool.ErrDenied
	}
	if len(args) > tool.MaxArgumentBytes {
		return nil, tool.ErrLimit
	}
	t, err := e.registry.Get(name)
	if err != nil {
		return nil, err
	}
	normalized, err := t.Validate(args)
	if err != nil {
		return nil, err
	}
	var invocation tool.Invocation
	if aware, ok := t.(tool.PermissionTool); ok {
		invocation, err = aware.PrepareWithPermissions(ctx, scope, normalized, permissions)
	} else {
		invocation, err = t.Prepare(ctx, scope, normalized)
	}
	if err != nil {
		if invocation != nil {
			err = errors.Join(err, invocation.Close())
		}
		return nil, err
	}
	if invocation == nil {
		return nil, tool.ErrInvalid
	}
	var workspace *tool.Scope
	if scope.ID != "" {
		workspace = new(scope)
	}
	target := invocation.Target()
	if target != nil {
		target = new(*target)
	}
	definition := t.Definition()
	definition.Parameters = slices.Clone(definition.Parameters)
	definition.Effects = slices.Clone(definition.Effects)
	plan, err := invocationPlan(invocation)
	if err != nil {
		return nil, errors.Join(err, invocation.Close())
	}
	details, err := invocationSnapshot(invocation)
	if err != nil {
		return nil, errors.Join(err, invocation.Close())
	}
	effects := slices.Clone(definition.Effects)
	if plan != nil {
		// 未分类工具不能借额外的 write/read 声明降低原有审批要求。
		if len(effects) == 0 {
			effects = append(effects, tool.EffectUnknown)
		}
		for _, effect := range plan.AdditionalEffects {
			if !effect.Valid() {
				return nil, errors.Join(tool.ErrInvalid, invocation.Close())
			}
			if !slices.Contains(effects, effect) {
				effects = append(effects, effect)
			}
		}
	}
	effective := definition
	effective.Effects = effects
	request, err := json.Marshal(Operation{Tool: effective, Workspace: workspace, Target: target, Plan: plan, Details: details, Arguments: normalized, Summary: definition.Description})
	if err != nil {
		return nil, errors.Join(err, invocation.Close())
	}
	hash := sha256.Sum256(request)
	var boundary *tool.CommandBoundary
	if command, ok := invocation.(tool.CommandInvocation); ok {
		boundary = new(command.CommandBoundary())
	}
	return &Prepared{invocation: invocation, target: target, plan: plan, details: details, effects: effects, instance: t, definition: definition, request: request, hash: hex.EncodeToString(hash[:]), boundary: boundary}, nil
}

func invocationSnapshot(invocation tool.Invocation) (json.RawMessage, error) {
	detailed, ok := invocation.(tool.SnapshotInvocation)
	if !ok {
		return nil, nil
	}
	raw := detailed.Snapshot()
	var value map[string]json.RawMessage
	if len(raw) > tool.MaxArgumentBytes || json.Unmarshal(raw, &value) != nil || len(value) == 0 {
		return nil, tool.ErrInvalid
	}
	// 归一化对象顺序并持有独立副本；执行前比较同一规范化结果。
	return json.Marshal(value)
}

func invocationPlan(invocation tool.Invocation) (*tool.Plan, error) {
	planned, ok := invocation.(tool.PlannedInvocation)
	if !ok {
		return nil, nil
	}
	plan := planned.Plan()
	plan.Targets = slices.Clone(plan.Targets)
	plan.AdditionalEffects = slices.Clone(plan.AdditionalEffects)
	if len(plan.Targets) > 32 {
		return nil, tool.ErrLimit
	}
	for _, target := range plan.Targets {
		if !filepath.IsAbs(target.Path) || !filepath.IsAbs(target.RequestedPath) || target.Kind == "" {
			return nil, tool.ErrInvalid
		}
	}
	return &plan, nil
}

// Execute 的 approvedHash 来自权限判定或已批准记录，工具结果必须是有界有效 JSON。
func (e *Executor) Execute(ctx context.Context, p *Prepared, approvedHash string) (tool.Result, error) {
	return e.ExecuteStream(ctx, p, approvedHash, nil)
}

// ExecuteStream 沿用普通工具的权限边界，并串行转交输出；旧工具无需实现流式接口。
func (e *Executor) ExecuteStream(ctx context.Context, p *Prepared, approvedHash string, emit func(tool.OutputChunk) error) (tool.Result, error) {
	notApplied := tool.Result{Outcome: tool.OutcomeNotApplied}
	if p == nil || p.hash != approvedHash {
		return notApplied, tool.ErrDenied
	}
	target := p.invocation.Target()
	if (target == nil) != (p.target == nil) || (target != nil && *target != *p.target) {
		return notApplied, tool.ErrDenied
	}
	plan, err := invocationPlan(p.invocation)
	if err != nil {
		return notApplied, err
	}
	if (plan == nil) != (p.plan == nil) || (plan != nil && (!slices.Equal(plan.Targets, p.plan.Targets) || !slices.Equal(plan.AdditionalEffects, p.plan.AdditionalEffects))) {
		return notApplied, tool.ErrDenied
	}
	details, err := invocationSnapshot(p.invocation)
	if err != nil {
		return notApplied, err
	}
	if !bytes.Equal(details, p.details) {
		return notApplied, tool.ErrDenied
	}
	if p.boundary != nil {
		command, ok := p.invocation.(tool.CommandInvocation)
		if !ok || command.CommandBoundary() != *p.boundary {
			return notApplied, tool.ErrDenied
		}
	}
	definition := p.instance.Definition()
	latest, err := json.Marshal(definition)
	if err != nil {
		return notApplied, err
	}
	original, err := json.Marshal(p.definition)
	if err != nil {
		return notApplied, err
	}
	if string(latest) != string(original) {
		return notApplied, tool.ErrDenied
	}
	call, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	if err := call.Err(); err != nil {
		return notApplied, err
	}
	var result tool.Result
	var streamErr error
	var outputMu sync.Mutex
	var count, outputBytes int
	if streaming, ok := p.invocation.(tool.StreamingInvocation); ok && emit != nil {
		result, err = streaming.ExecuteStream(call, func(chunk tool.OutputChunk) error {
			outputMu.Lock()
			defer outputMu.Unlock()
			if streamErr != nil {
				return streamErr
			}
			if call.Err() != nil {
				streamErr = call.Err()
			} else if (chunk.Stream != "stdout" && chunk.Stream != "stderr") || len(chunk.Data) == 0 || len(chunk.Data) > tool.MaxOutputChunkBytes {
				streamErr = tool.ErrInvalid
			} else if count >= tool.MaxOutputEvents || outputBytes+len(chunk.Data) > tool.MaxOutputBytes {
				streamErr = tool.ErrLimit
			} else {
				count++
				outputBytes += len(chunk.Data)
				chunk.Sequence = int64(count)
				streamErr = emit(chunk)
			}
			return streamErr
		})
	} else {
		result, err = p.invocation.Execute(call)
	}
	err = errors.Join(err, streamErr)
	if !result.Outcome.Valid() {
		result.Outcome = tool.UnconfirmedOutcome(p.effects)
		err = errors.Join(err, tool.ErrInvalid)
	}
	err = errors.Join(err, call.Err())
	if err == nil && result.Outcome == tool.OutcomeUnknown {
		err = tool.ErrUnknownOutcome
	} else if err == nil && result.Outcome == tool.OutcomePartial {
		err = tool.ErrPartialOutcome
	}
	// 输出不可保存时仍保留独立事实，不能把已发生的操作改成未执行。
	if len(result.Output) > tool.MaxResultBytes {
		result.Output = nil
		err = errors.Join(err, tool.ErrLimit)
	} else if (result.Output != nil || err == nil) && !json.Valid(result.Output) {
		result.Output = nil
		err = errors.Join(err, tool.ErrInvalid)
	}
	if len(result.Images) > 8 {
		result.Images = nil
		err = errors.Join(err, tool.ErrLimit)
	}
	if len(result.Images) > 0 {
		metadata, metadataErr := json.Marshal(result.Images)
		if metadataErr != nil || len(metadata)+len(result.Output) > tool.MaxResultBytes {
			result.Images = nil
			err = errors.Join(err, tool.ErrLimit, metadataErr)
		}
	}
	for _, block := range result.Images {
		if block.Type != "image" || block.AttachmentRef == "" || block.Path != "" {
			result.Images = nil
			err = errors.Join(err, tool.ErrInvalid)
			break
		}
	}
	return result, err
}
