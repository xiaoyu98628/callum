package run

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/sys/windows"

	"github.com/xiaoyu98628/callum/internal/tool"
)

const sandboxJournalVersion = 1
const maxSandboxEntries = 65536
const maxSandboxJournalBytes = 16 << 20
const maxSandboxRoots = 4 // cwd、程序目录、可选工作区及完整内置运行时树。

type sandboxRoot struct {
	Path     string `json:"path"`
	Identity string `json:"identity"`
	Write    bool   `json:"write"`
	Ambient  bool   `json:"ambient,omitempty"`
}

type sandboxACLEntry struct {
	Path      string `json:"path"`
	Identity  string `json:"identity"`
	Directory bool   `json:"directory"`
	Mask      uint32 `json:"mask"`
	Deny      bool   `json:"deny,omitempty"`
	SelfOnly  bool   `json:"self_only,omitempty"` // 祖先目录的删除保护不向普通子项传播。
}

type sandboxJournal struct {
	ProtectedDirectory string            `json:"protected_directory"`
	Version            int               `json:"version"`
	Name               string            `json:"name"`
	SID                string            `json:"sid"`
	Temporary          string            `json:"temporary"`
	Roots              []sandboxRoot     `json:"roots"`
	Entries            []sandboxACLEntry `json:"entries"`
}

type sandboxState struct {
	roots        []*fixedPath
	journal      sandboxJournal
	journalPath  string
	sid          *windows.SID
	token        windows.Token
	active       bool
	profileOwned bool
	aclTouched   bool
}

func (s *sandboxState) readableRoots() []string {
	var roots []string
	for _, root := range s.journal.Roots {
		roots = append(roots, root.Path)
	}
	return roots
}

func (s *sandboxState) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, root := range s.roots {
		if err := root.check(false); err != nil {
			return err
		}
	}
	return nil
}

func (s *sandboxState) close() error {
	var err error
	if s.active || s.token != 0 {
		err = s.cleanup()
	}
	for _, root := range s.roots {
		err = errors.Join(err, root.file.Close())
	}
	s.roots = nil
	return err
}

func (*fixedCommand) sandboxBackend() string { return "windows-appcontainer" }

func (p *fixedCommand) configureSandbox(a arguments, permissions tool.ExecutionPermissions, protection tool.ProtectedData) error {
	if permissions.SandboxMode != "" && !permissions.SandboxMode.Valid() {
		return tool.ErrDenied
	}
	p.boundary = tool.CommandBoundary{Mode: permissions.SandboxMode, Escalated: a.SandboxPermissions == sandboxEscalated, Risk: commandRisk(p.command)}
	if permissions.SandboxMode == tool.SandboxFullAccess {
		p.boundary.Escalated = false
		return nil
	}
	projectWrite := a.SandboxPermissions == sandboxProjectWrite
	if projectWrite && (permissions.SandboxMode != tool.SandboxWorkspaceWrite || p.workspace == nil) {
		return tool.ErrDenied
	}
	if p.boundary.Escalated || permissions.SandboxMode == "" {
		return nil
	}
	if protection.Directory == "" {
		return fmt.Errorf("Windows 沙箱需要持有应用数据目录的恢复记录: %w", tool.ErrUnavailable)
	}
	for _, path := range []string{p.directory.actual, p.executable.actual, protection.Directory, os.TempDir()} {
		volume := filepath.VolumeName(path)
		if len(volume) != 2 || volume[1] != ':' {
			return fmt.Errorf("Windows 受限命令需要本地卷路径: %w", tool.ErrUnavailable)
		}
	}
	for _, proc := range []*windows.LazyProc{createAppContainerProfile, deleteAppContainerProfile, deriveAppContainerSID, createRestrictedToken} {
		if err := proc.Find(); err != nil {
			return errors.Join(tool.ErrUnavailable, err)
		}
	}
	var err error
	p.protectedDirectory, err = pinPath(protection.Directory, true)
	if err != nil {
		return err
	}
	protection.Directory = p.protectedDirectory.actual
	p.protection.Directory = protection.Directory
	p.temporaryParent, err = pinPath(os.TempDir(), true)
	if err != nil {
		return err
	}
	if protection.Contains(p.temporaryParent.actual) || (projectWrite && protection.Contains(p.workspace.actual)) {
		return tool.ErrDenied
	}
	p.temporaryPath = filepath.Join(p.temporaryParent.actual, "callum-command-"+rand.Text())
	p.boundary.Sandboxed, p.boundary.Mode = true, tool.SandboxReadOnly
	if projectWrite {
		p.boundary.Mode = tool.SandboxWorkspaceWrite
		p.boundary.CreatesWorkspaceDirectory = p.createsWorkspaceDirectory()
	}
	s := &p.sandboxState
	s.journal = sandboxJournal{Version: sandboxJournalVersion, Name: "callum.command." + rand.Text(), Temporary: p.temporaryPath, ProtectedDirectory: p.protectedDirectory.actual}
	paths := []string{p.directory.actual, filepath.Dir(p.executable.actual)}
	if p.workDirectory != nil {
		paths = append(paths, p.workDirectory.actual)
	}
	for _, root := range p.readOnlyDirectories {
		paths = append(paths, root.actual)
	}
	if p.workspace != nil {
		paths = append(paths, p.workspace.actual)
	}
	for _, path := range paths {
		if slices.ContainsFunc(s.roots, func(root *fixedPath) bool { return strings.EqualFold(root.actual, path) }) {
			continue
		}
		if protection.ReadProtected(path) {
			return tool.ErrDenied
		}
		root, err := pinPath(path, true)
		if err != nil {
			return err
		}
		s.roots = append(s.roots, root)
		write := projectWrite && p.workspace != nil && strings.EqualFold(root.actual, p.workspace.actual)
		write = write || (p.workDirectory != nil && strings.EqualFold(root.actual, p.workDirectory.actual))
		if write && isWindowsSystemDirectory(root.actual, p.environment) {
			return tool.ErrDenied
		}
		ambient := !write && (isWindowsSystemDirectory(root.actual, p.environment) || ambientWindowsRead(root.actual))
		s.journal.Roots = append(s.journal.Roots, sandboxRoot{Path: root.actual, Identity: root.identity, Write: write, Ambient: ambient})
	}
	if len(s.journal.Roots) > maxSandboxRoots {
		return tool.ErrLimit
	}
	profile, err := json.Marshal(struct {
		Policy string
		Roots  []sandboxRoot
	}{Policy: "windows-appcontainer-v1;no-network;write-restricted;private-temp", Roots: s.journal.Roots})
	if err != nil {
		return err
	}
	p.profile = string(profile)
	// 随机私有目录不进入权限指纹；实际读写根目录由快照绑定。
	for _, key := range []string{"TMPDIR", "TMP", "TEMP", "GOCACHE", "GOTMPDIR", "npm_config_cache", "PIP_CACHE_DIR"} {
		p.environment = slices.DeleteFunc(p.environment, func(entry string) bool { name, _, _ := strings.Cut(entry, "="); return strings.EqualFold(name, key) })
		value := p.temporaryPath
		if key == "GOCACHE" {
			value = filepath.Join(value, "go-build")
		}
		if key == "npm_config_cache" {
			value = filepath.Join(value, "npm-cache")
		}
		p.environment = append(p.environment, key+"="+value)
	}
	return nil
}

func (p *fixedCommand) setupExecution(ctx context.Context) (cleanup func() error, retErr error) {
	if !p.boundary.Sandboxed {
		return func() error { return nil }, nil
	}
	s := &p.sandboxState
	if s.active {
		return nil, tool.ErrDenied
	}
	// 恢复目录位于模型不可访问的应用数据目录内。每次执行拥有独立 SID 与日志。
	directory := filepath.Join(p.protectedDirectory.actual, "command-sandboxes")
	if err := ensureSandboxJournalDirectory(directory); err != nil {
		return nil, err
	}
	s.journalPath = filepath.Join(directory, s.journal.Name+".json")
	sid, err := sandboxSID(s.journal.Name)
	if err != nil {
		return nil, err
	}
	s.sid, s.journal.SID = sid, sid.String()
	if err := os.Mkdir(p.temporaryPath, 0700); err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			cleanupErr := s.cleanup()
			if cleanupErr != nil {
				retErr = errors.Join(retErr, tool.ErrUnknownOutcome, cleanupErr)
			}
		}
	}()
	if err := secureNewSandboxDirectory(p.temporaryPath); err != nil {
		return nil, err
	}
	if err := p.collectSandboxACLs(ctx); err != nil {
		return nil, err
	}
	if err := writeSandboxJournal(s.journalPath, s.journal); err != nil {
		return nil, err
	}
	s.active = true
	s.profileOwned, err = newSandboxProfile(s.journal.Name, sid)
	if err != nil {
		return nil, err
	}
	s.token, err = sandboxRestrictedToken(sid)
	if err != nil {
		return nil, err
	}
	if err := withSandboxACLLock(func() error {
		s.aclTouched = true
		for _, entry := range s.journal.Entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := updateSandboxACL(entry, sid, false); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return s.cleanup, nil
}

func (p *fixedCommand) collectSandboxACLs(ctx context.Context) error {
	s := &p.sandboxState
	entries := map[string]sandboxACLEntry{}
	collect := func(root string, write, block bool) error {
		return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if block && !p.protection.ReadProtected(path) {
				return filepath.SkipDir
			}
			// 应用数据保留独立拒绝条目，遍历不会进入其中。
			protected := p.protection.ReadProtected(path)
			if !block && !strings.EqualFold(path, root) && protected {
				return filepath.SkipDir
			}
			if !block && protected {
				return tool.ErrDenied
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.IsDir() && !info.Mode().IsRegular() {
				return tool.ErrDenied
			}
			mask := uint32(windows.FILE_GENERIC_READ | windows.FILE_GENERIC_EXECUTE)
			deny := block || protectedSandboxPath(path) || p.protection.ReadOnlyContains(path)
			if write && !deny {
				mask |= windows.FILE_GENERIC_WRITE
				// 应用数据的祖先目录不能被重命名或删除。
				if !strings.EqualFold(path, root) && !p.protection.ProtectsDescendant(path) {
					mask |= windows.DELETE
				}
			}
			if deny {
				mask = windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA | windows.FILE_WRITE_EA | windows.FILE_WRITE_ATTRIBUTES | windows.DELETE | windows.WRITE_DAC | windows.WRITE_OWNER | sandboxDeleteChild
			}
			if block {
				mask = sandboxFileAllAccess
			}
			identity, err := sandboxPathIdentity(path)
			if err != nil {
				return err
			}
			key := strings.ToLower(path)
			old, exists := entries[key]
			if exists && !deny {
				mask |= old.Mask
			}
			selfOnly := block && info.IsDir() && p.protection.ProtectsDescendant(path)
			if selfOnly {
				// 应用数据根目录保留禁止列举等权限，允许路径穿越至单独登记的 skills 子树；拒绝不向该子树继承。
				mask &^= windows.FILE_TRAVERSE
			}
			entries[key] = sandboxACLEntry{Path: path, Identity: identity, Directory: info.IsDir(), Mask: mask, Deny: deny, SelfOnly: selfOnly}
			if !block && p.protection.ReadOnlyContains(path) {
				// 只读资源同时需要读取授权和独立写拒绝，不能只创建拒绝条目。
				entries[key+"\x00readonly-read"] = sandboxACLEntry{Path: path, Identity: identity, Directory: info.IsDir(), Mask: windows.FILE_GENERIC_READ | windows.FILE_GENERIC_EXECUTE}
			}
			if !deny && info.IsDir() && p.protection.ProtectsDescendant(path) {
				// 工作区父目录的继承 DELETE 授权不能抵消资源祖先的固定路径保护。
				entries[key+"\x00ancestor"] = sandboxACLEntry{Path: path, Identity: identity, Directory: true, Mask: windows.DELETE | sandboxDeleteChild, Deny: true, SelfOnly: true}
			}
			if len(entries) > maxSandboxEntries {
				return tool.ErrLimit
			}
			return nil
		})
	}
	for _, root := range s.journal.Roots {
		// 系统目录沿用 Windows 已有 AppContainer 读取授权；用户安装的程序目录显式授权。
		if root.Ambient {
			continue
		}
		if err := collect(root.Path, root.Write, false); err != nil {
			return err
		}
	}
	if err := collect(p.temporaryPath, true, false); err != nil {
		return err
	}
	if err := collect(p.protectedDirectory.actual, false, true); err != nil {
		return err
	}
	for _, entry := range entries {
		s.journal.Entries = append(s.journal.Entries, entry)
	}
	// 拒绝先于授权建立。系统 API 不向已有子对象自动传播 ACL，避免改写符号链接目标。
	slices.SortFunc(s.journal.Entries, func(a, b sandboxACLEntry) int {
		if a.Deny != b.Deny {
			if a.Deny {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Path, b.Path)
	})
	return nil
}

func protectedSandboxPath(path string) bool {
	for part := range strings.SplitSeq(filepath.ToSlash(path), "/") {
		if slices.Contains([]string{".git", ".codex", ".agents", ".aws"}, strings.ToLower(part)) {
			return true
		}
	}
	return false
}

func isWindowsSystemDirectory(path string, _ []string) bool {
	root, err := windows.GetWindowsDirectory()
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(strings.ToLower(root), strings.ToLower(path))
	return err == nil && filepath.IsLocal(relative)
}

// 仅系统 cmd /d /c mkdir 加单个普通名称可进入单层目录创建审核。
func (p *fixedCommand) createsWorkspaceDirectory() bool {
	if p.workspace == nil || len(p.command) != 5 || !strings.EqualFold(p.command[1], "/d") || !strings.EqualFold(p.command[2], "/c") || !strings.EqualFold(p.command[3], "mkdir") {
		return false
	}
	name := p.command[4]
	if !filepath.IsLocal(name) || name == "" || strings.HasPrefix(name, "-") {
		return false
	}
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' {
			return false
		}
	}
	systemRoot, err := windows.GetWindowsDirectory()
	if err != nil {
		return false
	}
	actual, resolveErr := filepath.EvalSymlinks(filepath.Join(systemRoot, "System32", "cmd.exe"))
	trusted := resolveErr == nil && strings.EqualFold(actual, p.executable.actual)
	relative, err := filepath.Rel(strings.ToLower(p.workspace.actual), strings.ToLower(p.directory.actual))
	return trusted && err == nil && filepath.IsLocal(relative)
}
