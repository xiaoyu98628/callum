package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/registry"
)

// Operation 是服务端生成的审批快照，与实际执行使用相同的规范化参数。
type Operation struct {
	Tool      tool.Definition `json:"tool"`
	Workspace tool.Scope      `json:"workspace"`
	Arguments json.RawMessage `json:"arguments"`
	Summary   string          `json:"summary"`
}

// Prepared 持有固定的操作和根句柄，创建者必须 Close；等待审批期间不能替换操作。
type Prepared struct {
	root       *os.Root
	instance   tool.Tool
	arguments  json.RawMessage
	definition tool.Definition
	request    json.RawMessage
	hash       string
}

func (p *Prepared) Request() json.RawMessage { return slices.Clone(p.request) }
func (p *Prepared) Hash() string             { return p.hash }
func (p *Prepared) Definition() tool.Definition {
	d := p.definition
	d.Parameters = slices.Clone(d.Parameters)
	d.Effects = slices.Clone(d.Effects)
	return d
}
func (p *Prepared) Close() error { return p.root.Close() }

// Executor 在审批之前校验参数并固定工作区，审批之后再次检查操作指纹。
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if scope.ID == "" || !filepath.IsAbs(scope.RootPath) {
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
	root, err := os.OpenRoot(scope.RootPath)
	if err != nil {
		return nil, tool.ErrUnavailable
	}
	definition := t.Definition()
	definition.Parameters = slices.Clone(definition.Parameters)
	definition.Effects = slices.Clone(definition.Effects)
	request, err := json.Marshal(Operation{Tool: definition, Workspace: scope, Arguments: normalized, Summary: definition.Description})
	if err != nil {
		return nil, errors.Join(err, root.Close())
	}
	hash := sha256.Sum256(request)
	return &Prepared{root: root, instance: t, arguments: normalized, definition: definition, request: request, hash: hex.EncodeToString(hash[:])}, nil
}

// Execute 的 approvedHash 来自权限判定或已批准记录，工具结果必须是有界有效 JSON。
func (e *Executor) Execute(ctx context.Context, p *Prepared, approvedHash string) (json.RawMessage, error) {
	if p == nil || p.hash != approvedHash {
		return nil, tool.ErrDenied
	}
	definition := p.instance.Definition()
	latest, err := json.Marshal(definition)
	if err != nil {
		return nil, err
	}
	original, err := json.Marshal(p.definition)
	if err != nil {
		return nil, err
	}
	if string(latest) != string(original) {
		return nil, tool.ErrDenied
	}
	call, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	if err := call.Err(); err != nil {
		return nil, err
	}
	result, err := p.instance.Execute(call, p.root, p.arguments)
	if err == nil {
		err = call.Err()
	}
	if len(result) > tool.MaxResultBytes {
		return nil, tool.ErrLimit
	}
	if err == nil && !json.Valid(result) {
		return nil, tool.ErrInvalid
	}
	return result, err
}
