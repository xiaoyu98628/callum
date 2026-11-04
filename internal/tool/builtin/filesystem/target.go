package filesystem

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/xiaoyu98628/callum/internal/tool"
)

// invocation 在审批前只固定句柄和元数据，不读取文件正文或目录内容。
// 单文件持有文件句柄，目录持有仅覆盖目标目录的 Root，不授权相邻资源。
type invocation struct {
	name          string
	args          arguments
	target        tool.Target
	info          os.FileInfo
	root          *os.Root
	file          *os.File
	scope         tool.Scope
	workspace     string
	workspaceInfo os.FileInfo
	closeOnce     sync.Once
	closeErr      error
}

func prepare(ctx context.Context, scope tool.Scope, name string, a arguments) (_ tool.Invocation, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	requested := a.Path
	if !filepath.IsAbs(requested) {
		if scope.ID == "" {
			return nil, tool.ErrPathBase
		}
		requested = filepath.Join(scope.RootPath, requested)
	}
	resolved, err := filepath.EvalSymlinks(requested)
	if err != nil {
		return nil, tool.ErrUnavailable
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, tool.ErrUnavailable
	}
	p := &invocation{name: name, args: a, info: info, scope: scope,
		target: tool.Target{RequestedPath: requested, Path: resolved, Kind: "file"}}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, p.Close())
		}
	}()
	if info.IsDir() {
		p.target.Kind = "directory"
	} else if !info.Mode().IsRegular() || name == "list_directory" {
		return nil, tool.ErrUnavailable
	}
	if name == "read_file" && !info.Mode().IsRegular() {
		return nil, tool.ErrUnavailable
	}
	if scope.ID != "" {
		p.workspace, err = filepath.EvalSymlinks(scope.RootPath)
		if err != nil {
			return nil, tool.ErrUnavailable
		}
		p.workspaceInfo, err = os.Stat(p.workspace)
		if err != nil || !p.workspaceInfo.IsDir() {
			return nil, tool.ErrUnavailable
		}
		rel, err := filepath.Rel(p.workspace, resolved)
		p.target.WithinWorkspace = err == nil && filepath.IsLocal(rel)
	}
	// 工作区内目标必须经工作区 Root 打开；即使路径在此时被替换为越界链接，
	// 也不能因先前的路径分类而获得自动放行后的越界读取能力。
	base, relative := filepath.Dir(resolved), filepath.Base(resolved)
	if info.IsDir() {
		base, relative = resolved, "."
	}
	if p.target.WithinWorkspace {
		base = p.workspace
		relative, err = filepath.Rel(base, resolved)
		if err != nil {
			return nil, tool.ErrDenied
		}
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		return nil, tool.ErrUnavailable
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()
	if p.target.WithinWorkspace {
		opened, err := root.Stat(".")
		if err != nil || !os.SameFile(opened, p.workspaceInfo) {
			return nil, tool.ErrDenied
		}
	}
	var opened os.FileInfo
	if info.IsDir() {
		p.root, err = root.OpenRoot(relative)
		if err == nil {
			opened, err = p.root.Stat(".")
		}
	} else {
		p.file, err = root.OpenFile(relative, os.O_RDONLY|nonblock, 0)
		if err == nil {
			opened, err = p.file.Stat()
		}
	}
	if err != nil || !os.SameFile(info, opened) {
		return nil, tool.ErrDenied
	}
	if err := p.check(ctx); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *invocation) Target() *tool.Target { return new(p.target) }

// check 检查原始路径、目标身份及工作区是否仍与审批时一致。
// 读取始终使用固定句柄；检查之后发生的路径替换也不能把读取重定向到别的文件。
func (p *invocation) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(p.target.RequestedPath)
	if err != nil || resolved != p.target.Path {
		return tool.ErrDenied
	}
	current, err := os.Stat(resolved)
	if err != nil || !os.SameFile(current, p.info) {
		return tool.ErrDenied
	}
	if p.scope.ID != "" {
		workspace, err := filepath.EvalSymlinks(p.scope.RootPath)
		if err != nil || workspace != p.workspace {
			return tool.ErrDenied
		}
		current, err := os.Stat(workspace)
		if err != nil || !os.SameFile(current, p.workspaceInfo) {
			return tool.ErrDenied
		}
	}
	return nil
}

func (p *invocation) Execute(ctx context.Context) (tool.Result, error) {
	output, err := p.execute(ctx)
	return tool.Result{Output: output, Outcome: tool.OutcomeNone}, err
}

func (p *invocation) execute(ctx context.Context) (json.RawMessage, error) {
	if err := p.check(ctx); err != nil {
		return nil, err
	}
	a := p.args
	switch p.name {
	case "read_file":
		data, err := fileText(ctx, p.file)
		if err != nil {
			return nil, err
		}
		a.Path = p.target.Path
		return read(a, data)
	case "list_directory":
		a.Path = "."
		return list(ctx, p.root, a)
	case "search_text":
		a.Path = "."
		return search(ctx, p.root, p.file, a, p.target.Path)
	}
	return nil, tool.ErrUnavailable
}

func (p *invocation) Close() error {
	p.closeOnce.Do(func() {
		if p.root != nil {
			p.closeErr = p.root.Close()
		}
		if p.file != nil {
			p.closeErr = errors.Join(p.closeErr, p.file.Close())
		}
	})
	return p.closeErr
}
