package patch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/internal/fileaccess"
)

type patchChange struct {
	Path           string       `json:"path"`
	Action         string       `json:"action"`
	Outcome        tool.Outcome `json:"outcome"`
	BeforeSHA256   string       `json:"before_sha256,omitempty"`
	IntendedSHA256 string       `json:"intended_sha256,omitempty"`
	ObservedSHA256 string       `json:"observed_sha256,omitempty"`
}
type patchReport struct {
	Changes            []patchChange `json:"changes"`
	CreatedDirectories []string      `json:"created_directories"`
	RecoveryFiles      []string      `json:"recovery_files"`
}
type patchArtifact struct {
	root       *os.Root
	name, path string
	info       os.FileInfo
	removed    bool
}

func textSHA256(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func patchSnapshot(ctx context.Context, file *os.File) ([]byte, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, tool.ErrUnavailable
	}
	return fileaccess.ReadText(ctx, file)
}

func (p *patchInvocation) Execute(ctx context.Context) (result tool.Result, retErr error) {
	result.Outcome = tool.OutcomeNotApplied
	if !p.started.CompareAndSwap(false, true) {
		return result, tool.ErrDenied
	}
	release, err := p.owner.acquire(ctx, p)
	if err != nil {
		return result, err
	}
	defer release()
	report := patchReport{Changes: make([]patchChange, len(p.files)), CreatedDirectories: []string{}, RecoveryFiles: []string{}}
	for i, entry := range p.files {
		report.Changes[i] = patchChange{Path: entry.target.Path, Action: entry.patch.action, Outcome: tool.OutcomeNotApplied}
	}
	defer func() {
		result.Outcome = report.outcome(retErr)
		output, err := json.Marshal(report)
		result.Output = output
		retErr = errors.Join(retErr, err)
	}()
	// 全部版本及上下文通过后才创建目录、暂存文件或修改任何目标。
	before, after := make([][]byte, len(p.files)), make([][]byte, len(p.files))
	for i, entry := range p.files {
		if err := entry.check(ctx, p); err != nil {
			return result, err
		}
		if entry.fixed != nil {
			data, err := patchSnapshot(ctx, entry.fixed.File())
			if err != nil {
				return result, err
			}
			report.Changes[i].BeforeSHA256 = textSHA256(data)
			if report.Changes[i].BeforeSHA256 != entry.target.ExpectedSHA256 {
				report.Changes[i].ObservedSHA256 = report.Changes[i].BeforeSHA256
				return result, tool.ErrConflict
			}
			before[i] = data
			after[i], err = applyText(data, entry.patch)
			if err != nil {
				return result, err
			}
		} else {
			after[i] = []byte(entry.patch.content)
		}
		report.Changes[i].IntendedSHA256 = textSHA256(after[i])
	}
	for i, entry := range p.files {
		if err := entry.check(ctx, p); err != nil {
			return result, err
		}
		if err := entry.openParent(ctx, p, &report); err != nil {
			return result, err
		}
		if err := p.writeTarget(ctx, entry, before[i], after[i], &report.Changes[i], &report); err != nil {
			return result, err
		}
	}
	return result, ctx.Err()
}

func (r patchReport) outcome(err error) tool.Outcome {
	if errors.Is(err, tool.ErrUnknownOutcome) {
		return tool.OutcomeUnknown
	}
	applied, partial := 0, false
	for _, change := range r.Changes {
		switch change.Outcome {
		case tool.OutcomeUnknown:
			return tool.OutcomeUnknown
		case tool.OutcomePartial:
			partial = true
		case tool.OutcomeApplied:
			applied++
		}
	}
	if applied == len(r.Changes) && !partial {
		return tool.OutcomeApplied
	}
	if partial || applied > 0 || (err != nil && (len(r.CreatedDirectories) > 0 || len(r.RecoveryFiles) > 0)) {
		return tool.OutcomePartial
	}
	return tool.OutcomeNotApplied
}

func (t *patchTarget) openParent(ctx context.Context, p *patchInvocation, report *patchReport) (retErr error) {
	current, owned := t.root, false
	prefix := t.basePath
	defer func() {
		if owned {
			retErr = errors.Join(retErr, current.Close())
		}
	}()
	if t.parentRelative != "." {
		for component := range strings.SplitSeq(t.parentRelative, string(filepath.Separator)) {
			if err := t.check(ctx, p); err != nil {
				return err
			}
			if err := checkPatchDirectory(current, prefix); err != nil {
				return err
			}
			// 每一级都从已经固定的父句柄继续，不能按复合路径重走可能被替换的祖先。
			created := false
			if err := current.Mkdir(component, 0700); err == nil {
				created = true
				report.CreatedDirectories = append(report.CreatedDirectories, filepath.Join(prefix, component))
			} else if !errors.Is(err, os.ErrExist) {
				return err
			}
			info, err := current.Lstat(component)
			if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return tool.ErrDenied
			}
			next, err := current.OpenRoot(component)
			if err != nil {
				return tool.ErrDenied
			}
			opened, err := next.Stat(".")
			if err != nil || !os.SameFile(opened, info) {
				return errors.Join(tool.ErrDenied, next.Close())
			}
			if created {
				file, err := next.Open(".")
				if err != nil {
					return errors.Join(err, next.Close())
				}
				secureErr := protectPatchDirectory(file)
				closeErr := file.Close()
				if err := errors.Join(secureErr, closeErr); err != nil {
					return errors.Join(err, next.Close())
				}
			}
			if owned {
				owned = false
				if err := current.Close(); err != nil {
					return errors.Join(err, next.Close())
				}
			}
			current, owned = next, true
			prefix = filepath.Join(prefix, component)
		}
	}
	if !owned {
		var err error
		current, err = t.root.OpenRoot(".")
		if err != nil {
			return tool.ErrDenied
		}
	}
	t.parent, owned = current, false // 句柄交给调用对象，所有退出分支由 Close 统一释放。
	return t.checkParent()
}
func (t *patchTarget) checkParent() error {
	return checkPatchDirectory(t.parent, filepath.Dir(t.target.Path))
}
func checkPatchDirectory(root *os.Root, path string) error {
	opened, err := root.Stat(".")
	if err != nil {
		return tool.ErrDenied
	}
	current, err := os.Stat(path)
	if err != nil || !os.SameFile(opened, current) {
		return tool.ErrDenied
	}
	return nil
}

func (a *patchArtifact) remove() error {
	if a.info == nil {
		return tool.ErrUnknownOutcome
	}
	current, err := a.root.Lstat(a.name)
	if errors.Is(err, os.ErrNotExist) {
		a.removed = true
		return tool.ErrConflict
	}
	if err != nil {
		return err
	}
	if !os.SameFile(current, a.info) {
		return tool.ErrConflict
	}
	if err := a.root.Remove(a.name); err != nil {
		return err
	}
	a.removed = true
	return nil
}

func privatePatchCopy(ctx context.Context, root *os.Root, path string, data []byte) (_ *patchArtifact, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	name := filepath.Base(path)
	file, err := fileaccess.OpenFile(root, name, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if errors.Is(err, os.ErrExist) {
		return nil, tool.ErrConflict
	}
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return &patchArtifact{root: root, name: name, path: path}, errors.Join(tool.ErrUnknownOutcome, err, file.Close())
	}
	a := &patchArtifact{root: root, name: name, path: path, info: info}
	defer func() { retErr = errors.Join(retErr, file.Close()) }()
	if err := protectPatchFile(file); err != nil {
		return a, err
	}
	if _, err := file.Write(data); err != nil {
		return a, err
	}
	if err := file.Sync(); err != nil {
		return a, err
	}
	return a, nil
}

func (p *patchInvocation) writeTarget(ctx context.Context, t *patchTarget, before, after []byte, change *patchChange, report *patchReport) (retErr error) {
	if err := t.check(ctx, p); err != nil {
		return err
	}
	if err := t.checkParent(); err != nil {
		return err
	}
	var artifacts []*patchArtifact
	defer func() {
		// 无目标修改时清理辅助文件；成功也清理。发生部分写入或结果未知时
		// 保留同步过的原文和预期内容，位置已经绑定在审批快照中。
		for _, a := range artifacts {
			if retErr == nil || change.Outcome == tool.OutcomeNotApplied {
				if err := a.remove(); err != nil {
					retErr = errors.Join(retErr, err)
				}
			}
			if !a.removed {
				report.RecoveryFiles = append(report.RecoveryFiles, a.path)
			}
		}
	}()
	copyFile := func(path string, data []byte) error {
		a, err := privatePatchCopy(ctx, t.parent, path, data)
		if a != nil {
			artifacts = append(artifacts, a)
		}
		return err
	}
	if t.fixed != nil {
		if err := copyFile(t.target.RecoveryPath, before); err != nil {
			return err
		}
	}
	if err := copyFile(t.target.StagingPath, after); err != nil {
		return err
	}
	if err := t.check(ctx, p); err != nil {
		return err
	}
	if err := t.checkParent(); err != nil {
		return err
	}
	flags := os.O_RDWR
	if t.fixed == nil {
		flags |= os.O_CREATE | os.O_EXCL
	}
	file, err := fileaccess.OpenFile(t.parent, t.name, flags, 0600)
	if errors.Is(err, os.ErrExist) {
		return tool.ErrConflict
	}
	if err != nil {
		return err
	}
	// 文件创建本身已经生效，空目标或后续权限失败也不能标成未执行。
	if t.fixed == nil {
		change.Outcome = tool.OutcomePartial
	}
	defer func() { retErr = errors.Join(retErr, file.Close()) }()
	info, err := file.Stat()
	if err != nil {
		change.Outcome = tool.OutcomeUnknown
		return err
	}
	if t.fixed != nil && !os.SameFile(info, t.fixed.Info()) {
		return tool.ErrDenied
	}
	if err := patchFileReady(file); err != nil {
		return err
	}
	if t.fixed == nil {
		if err := protectPatchFile(file); err != nil {
			return err
		}
	} else {
		current, err := patchSnapshot(ctx, file)
		if err != nil {
			return err
		}
		change.ObservedSHA256 = textSHA256(current)
		if change.ObservedSHA256 != t.target.ExpectedSHA256 {
			return tool.ErrConflict
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// 开始覆盖后完成这一有界文件的写入和截断，再观察取消；不在两者之间
	// 主动放弃，避免把取消扩大为本可避免的损坏。系统错误仍如实报告。
	n, writeErr := file.WriteAt(after, 0)
	if n > 0 {
		change.Outcome = tool.OutcomePartial
	}
	if writeErr == nil && n != len(after) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = file.Truncate(int64(len(after)))
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	observed, readErr := patchSnapshot(context.WithoutCancel(ctx), file)
	if readErr != nil {
		change.Outcome = tool.OutcomeUnknown
		return errors.Join(writeErr, readErr)
	}
	change.ObservedSHA256 = textSHA256(observed)
	switch change.ObservedSHA256 {
	case change.IntendedSHA256:
		change.Outcome = tool.OutcomeApplied
	case change.BeforeSHA256:
		// 即使内容恰好仍与原文一致，已经写过的前缀也可能更新元数据。
		// 不将已发生的写入重新解释为未执行。
		if change.Outcome != tool.OutcomeNotApplied {
			change.Outcome = tool.OutcomePartial
		}
	default:
		change.Outcome = tool.OutcomePartial
	}
	if writeErr != nil {
		return writeErr
	}
	if change.Outcome != tool.OutcomeApplied {
		return tool.ErrConflict
	}
	resolved, err := filepath.EvalSymlinks(t.target.RequestedPath)
	current, statErr := os.Stat(t.target.Path)
	if err != nil || resolved != t.target.Path || statErr != nil || !os.SameFile(current, info) || t.checkParent() != nil || p.checkWorkspace(context.WithoutCancel(ctx)) != nil {
		// 最终路径身份无法确认，仍保留已写入句柄的内容哈希及恢复文件。
		change.Outcome = tool.OutcomeUnknown
		return tool.ErrConflict
	}
	return ctx.Err()
}
