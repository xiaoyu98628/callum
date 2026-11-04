package patch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/internal/fileaccess"
)

type patchTarget struct {
	patch                patchFile
	target               tool.Target
	fixed                *fileaccess.Handle
	basePath             string
	baseInfo             os.FileInfo
	root                 *os.Root
	parent               *os.Root
	parentRelative, name string
	missing              []string
}

type patchInvocation struct {
	owner         *patchTool
	scope         tool.Scope
	workspace     string
	workspaceInfo os.FileInfo
	files         []*patchTarget
	plan          tool.Plan
	started       atomic.Bool
	closeOnce     sync.Once
	closeErr      error
	operationSeed string
}

func (*patchInvocation) Target() *tool.Target { return nil }
func (p *patchInvocation) Plan() tool.Plan {
	return tool.Plan{Targets: slices.Clone(p.plan.Targets), AdditionalEffects: slices.Clone(p.plan.AdditionalEffects)}
}
func (p *patchInvocation) prepare(ctx context.Context, args patchArguments, files []patchFile) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	canonical, err := json.Marshal(args)
	if err != nil {
		return err
	}
	p.operationSeed = string(canonical)
	if p.scope != (tool.Scope{}) {
		if p.scope.ID == "" || !filepath.IsAbs(p.scope.RootPath) {
			return tool.ErrDenied
		}
		var err error
		p.workspace, err = filepath.EvalSymlinks(p.scope.RootPath)
		if err != nil {
			return tool.ErrUnavailable
		}
		p.workspaceInfo, err = os.Stat(p.workspace)
		if err != nil || !p.workspaceInfo.IsDir() {
			return tool.ErrUnavailable
		}
	}
	versions := map[string]string{}
	for _, v := range args.BaseVersions {
		versions[v.Path] = v.SHA256
	}
	directories := map[string]bool{}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		entry := &patchTarget{patch: file}
		p.files = append(p.files, entry) // 此后任何失败由调用对象统一关闭已经固定的资源。
		if err := p.prepareTarget(ctx, entry, versions[file.path]); err != nil {
			return err
		}
		for _, other := range p.files[:len(p.files)-1] {
			if strings.EqualFold(other.target.Path, entry.target.Path) || samePatchFile(other, entry) {
				return tool.ErrConflict
			}
		}
		p.plan.Targets = append(p.plan.Targets, entry.target)
		for _, dir := range entry.missing {
			if directories[dir] {
				continue
			}
			directories[dir] = true
			if len(directories) > 16 {
				return tool.ErrLimit
			}
			p.plan.Targets = append(p.plan.Targets, tool.Target{RequestedPath: dir, Path: dir, Kind: "directory", Action: "create", WithinWorkspace: entry.target.WithinWorkspace})
		}
		// 不读取正文即可做保守分类：可能覆盖全部内容，或删除上限达到
		// 原文件一半且至少 4 KiB 时要求审批。真正内容和版本在批准后校验。
		if entry.fixed != nil && file.removedUpper > 0 && (file.removedUpper >= entry.fixed.Info().Size() || (file.removedUpper >= 4096 && file.removedUpper*2 >= entry.fixed.Info().Size())) {
			if !slices.Contains(p.plan.AdditionalEffects, tool.EffectDestructive) {
				p.plan.AdditionalEffects = append(p.plan.AdditionalEffects, tool.EffectDestructive)
			}
		}
		for _, path := range []string{entry.target.Path, entry.target.RequestedPath} {
			for part := range strings.SplitSeq(filepath.ToSlash(path), "/") {
				part = strings.ToLower(part)
				if part == ".git" || part == ".codex" || part == ".agents" {
					if !slices.Contains(p.plan.AdditionalEffects, tool.EffectUnknown) {
						p.plan.AdditionalEffects = append(p.plan.AdditionalEffects, tool.EffectUnknown)
					}
				}
			}
		}
	}
	encoded, err := json.Marshal(p.plan)
	if err != nil {
		return err
	}
	if len(encoded) > 24<<10 {
		return tool.ErrLimit
	}
	return p.checkWorkspace(ctx)
}

func (p *patchInvocation) prepareTarget(ctx context.Context, entry *patchTarget, hash string) error {
	requested := entry.patch.path
	if !filepath.IsAbs(requested) {
		if p.scope.ID == "" {
			return tool.ErrPathBase
		}
		requested = filepath.Join(p.scope.RootPath, requested)
	}
	if entry.patch.action == "update" {
		fixed, err := fileaccess.Prepare(ctx, p.scope, requested, fileaccess.File)
		if err != nil {
			return err
		}
		entry.fixed = fixed
		entry.target = *entry.fixed.Target()
		entry.basePath = filepath.Dir(entry.target.Path)
		entry.baseInfo, err = os.Stat(entry.basePath)
		if err != nil {
			return tool.ErrUnavailable
		}
		if entry.fixed.Info().Size() > 1<<20 {
			return tool.ErrLimit
		}
		if err := patchFileReady(entry.fixed.File()); err != nil {
			return err
		}
	} else {
		actual, base, info, missing, err := creationTarget(requested)
		if err != nil {
			return err
		}
		entry.target = tool.Target{RequestedPath: requested, Path: actual, Kind: "file"}
		entry.basePath, entry.baseInfo, entry.missing = base, info, missing
	}
	if len(entry.target.Path) > 1024 {
		return tool.ErrLimit
	}
	entry.target.WithinWorkspace = false
	if p.scope.ID != "" {
		rel, err := filepath.Rel(p.workspace, entry.target.Path)
		entry.target.WithinWorkspace = err == nil && filepath.IsLocal(rel)
	}
	entry.target.Action, entry.target.ExpectedSHA256 = entry.patch.action, hash
	// 辅助路径属于固定操作，必须可重复计算。随机名字会改变 request_hash，
	// 使同一未知操作的重试绕过重新确认；已有辅助文件用 O_EXCL 拒绝覆盖。
	sum := sha256.Sum256([]byte(p.operationSeed + "\x00" + p.scope.ID + "\x00" + entry.target.Path))
	id := hex.EncodeToString(sum[:16])
	parentPath := filepath.Dir(entry.target.Path)
	entry.target.RecoveryPath = filepath.Join(parentPath, ".callum-patch-"+id+"-before")
	entry.target.StagingPath = filepath.Join(parentPath, ".callum-patch-"+id+"-after")
	entry.name = filepath.Base(entry.target.Path)
	var err error
	entry.parentRelative, err = filepath.Rel(entry.basePath, parentPath)
	if err != nil || !filepath.IsLocal(entry.parentRelative) {
		return tool.ErrDenied
	}
	entry.root, err = p.openBase(entry)
	if err != nil {
		return err
	}
	return entry.check(ctx, p)
}

// creationTarget 固定尚不存在目标的最近现存祖先。Lstat 将悬空链接也视为
// 已有目标，不能把它当作空闲路径；返回的缺失目录只允许在执行阶段创建。
func creationTarget(requested string) (string, string, os.FileInfo, []string, error) {
	if _, err := os.Lstat(requested); err == nil {
		return "", "", nil, nil, tool.ErrConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", nil, nil, tool.ErrUnavailable
	}
	parent := filepath.Dir(requested)
	for depth := 0; ; depth++ {
		if depth > 16 {
			return "", "", nil, nil, tool.ErrLimit
		}
		info, err := os.Stat(parent)
		if err == nil {
			if !info.IsDir() {
				return "", "", nil, nil, tool.ErrConflict
			}
			resolved, err := filepath.EvalSymlinks(parent)
			if err != nil {
				return "", "", nil, nil, tool.ErrUnavailable
			}
			info, err = os.Stat(resolved)
			if err != nil {
				return "", "", nil, nil, tool.ErrUnavailable
			}
			rel, err := filepath.Rel(parent, requested)
			if err != nil || !filepath.IsLocal(rel) {
				return "", "", nil, nil, tool.ErrDenied
			}
			actual := filepath.Join(resolved, rel)
			var missing []string
			for dir := filepath.Dir(actual); dir != resolved; dir = filepath.Dir(dir) {
				missing = append(missing, dir)
			}
			slices.Reverse(missing)
			return actual, resolved, info, missing, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", "", nil, nil, tool.ErrUnavailable
		}
		// 不沿尚未解析的悬空符号链接创建父目录。
		if _, err := os.Lstat(parent); err == nil {
			return "", "", nil, nil, tool.ErrConflict
		}
		next := filepath.Dir(parent)
		if next == parent {
			return "", "", nil, nil, tool.ErrUnavailable
		}
		parent = next
	}
}

func (p *patchInvocation) openBase(entry *patchTarget) (_ *os.Root, retErr error) {
	var root *os.Root
	var err error
	if entry.target.WithinWorkspace {
		workspace, err := os.OpenRoot(p.workspace)
		if err != nil {
			return nil, tool.ErrUnavailable
		}
		defer func() { retErr = errors.Join(retErr, workspace.Close()) }()
		info, err := workspace.Stat(".")
		if err != nil || !os.SameFile(info, p.workspaceInfo) {
			return nil, tool.ErrDenied
		}
		rel, err := filepath.Rel(p.workspace, entry.basePath)
		if err != nil || !filepath.IsLocal(rel) {
			return nil, tool.ErrDenied
		}
		root, err = workspace.OpenRoot(rel)
	} else {
		root, err = os.OpenRoot(entry.basePath)
	}
	if err != nil {
		return nil, tool.ErrDenied
	}
	info, err := root.Stat(".")
	if err != nil || !os.SameFile(info, entry.baseInfo) {
		return nil, errors.Join(tool.ErrDenied, root.Close())
	}
	return root, nil
}
func (p *patchInvocation) checkWorkspace(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.scope.ID == "" {
		return nil
	}
	path, err := filepath.EvalSymlinks(p.scope.RootPath)
	if err != nil || path != p.workspace {
		return tool.ErrDenied
	}
	info, err := os.Stat(path)
	if err != nil || !os.SameFile(info, p.workspaceInfo) {
		return tool.ErrDenied
	}
	return nil
}
func (t *patchTarget) check(ctx context.Context, p *patchInvocation) error {
	if err := p.checkWorkspace(ctx); err != nil {
		return err
	}
	info, err := os.Stat(t.basePath)
	if err != nil || !os.SameFile(info, t.baseInfo) {
		return tool.ErrDenied
	}
	if t.fixed != nil {
		if err := t.fixed.Check(ctx); err != nil {
			return err
		}
		current, err := t.fixed.File().Stat()
		if err != nil || current.Size() != t.fixed.Info().Size() || current.Mode() != t.fixed.Info().Mode() || !current.ModTime().Equal(t.fixed.Info().ModTime()) {
			return tool.ErrConflict
		}
		return patchFileReady(t.fixed.File())
	}
	actual, _, _, _, err := creationTarget(t.target.RequestedPath)
	if err != nil {
		return err
	}
	if actual != t.target.Path {
		return tool.ErrDenied
	}
	return nil
}
func samePatchFile(a, b *patchTarget) bool {
	return a.fixed != nil && b.fixed != nil && os.SameFile(a.fixed.Info(), b.fixed.Info())
}
func (p *patchInvocation) overlaps(other *patchInvocation) bool {
	for _, a := range p.files {
		for _, b := range other.files {
			if strings.EqualFold(a.target.Path, b.target.Path) || samePatchFile(a, b) {
				return true
			}
		}
	}
	return false
}
func (p *patchInvocation) Close() error {
	p.closeOnce.Do(func() {
		for _, entry := range p.files {
			if entry.parent != nil {
				p.closeErr = errors.Join(p.closeErr, entry.parent.Close())
			}
			if entry.root != nil {
				p.closeErr = errors.Join(p.closeErr, entry.root.Close())
			}
			if entry.fixed != nil {
				p.closeErr = errors.Join(p.closeErr, entry.fixed.Close())
			}
		}
	})
	return p.closeErr
}
