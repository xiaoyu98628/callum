// Package fileaccess 管理文件工具共享的固定目标、句柄与有界文本读取。
package fileaccess

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/xiaoyu98628/callum/internal/tool"
)

// Handle 在审批前只固定句柄和元数据，不读取文件正文或目录内容。
// 创建者负责 Close；借用的文件与目录句柄不得由调用方单独关闭。
// 单文件持有文件句柄，目录持有仅覆盖目标目录的 Root，不授权相邻资源。
type Handle struct {
	target        tool.Target
	info          os.FileInfo
	protection    tool.ProtectedData
	root          *os.Root
	file          *os.File
	scope         tool.Scope
	workspace     string
	workspaceInfo os.FileInfo
	closeOnce     sync.Once
	closeErr      error
}

// Kind 限定允许固定的目标类型。
type Kind uint8

const (
	File            Kind = iota // 仅普通文件。
	Directory                   // 仅目录。
	FileOrDirectory             // 普通文件或目录。
)

// Prepare 固定已校验路径的目标及可选工作区身份，不读取正文。成功后由调用方关闭句柄。
func Prepare(ctx context.Context, scope tool.Scope, path string, kind Kind, protection tool.ProtectedData) (_ *Handle, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if kind > FileOrDirectory {
		return nil, tool.ErrInvalid
	}
	requested := path
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
	if protection.ReadProtected(resolved) {
		return nil, tool.ErrDenied
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, tool.ErrUnavailable
	}
	p := &Handle{info: info, scope: scope, protection: protection,
		target: tool.Target{RequestedPath: requested, Path: resolved, Kind: "file"}}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, p.Close())
		}
	}()
	if info.IsDir() {
		p.target.Kind = "directory"
	} else if !info.Mode().IsRegular() {
		return nil, tool.ErrUnavailable
	}
	if (kind == File && !info.Mode().IsRegular()) || (kind == Directory && !info.IsDir()) {
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
		p.file, err = OpenFile(root, relative, os.O_RDONLY, 0)
		if err == nil {
			opened, err = p.file.Stat()
		}
	}
	if err != nil || !os.SameFile(info, opened) {
		return nil, tool.ErrDenied
	}
	if err := p.Check(ctx); err != nil {
		return nil, err
	}
	return p, nil
}

// Target 返回固定目标元数据的副本。
func (p *Handle) Target() *tool.Target { return new(p.target) }

// Info 返回准备阶段记录的目标身份和元数据。
func (p *Handle) Info() os.FileInfo { return p.info }

// File 借出固定普通文件句柄；目录目标返回 nil。生命周期归 Handle 所有。
func (p *Handle) File() *os.File { return p.file }

// Root 借出固定目录句柄；文件目标返回 nil。生命周期归 Handle 所有。
func (p *Handle) Root() *os.Root { return p.root }

// Check 检查原始路径、目标身份及工作区是否仍与审批时一致。
// 读取始终使用固定句柄；检查之后发生的路径替换也不能把读取重定向到别的文件。
func (p *Handle) Check(ctx context.Context) error {
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
	if err := p.protection.CheckFile(current); err != nil {
		return err
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

// Close 释放持有的句柄，可重复调用。
func (p *Handle) Close() error {
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
