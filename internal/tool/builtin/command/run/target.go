package run

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/xiaoyu98628/callum/internal/tool"
)

type fixedPath struct {
	requested, actual string
	file              *os.File
	info              os.FileInfo
	identity          string
}

type fixedCommand struct {
	protection                                             tool.ProtectedData
	command                                                []string
	environment                                            []string
	executable, directory, workspace                       *fixedPath
	snapshot                                               json.RawMessage
	boundary                                               tool.CommandBoundary
	sandboxExecutable, temporaryParent, protectedDirectory *fixedPath
	temporaryPath, profile                                 string
	launchArguments                                        []string
	sandboxState                                           sandboxState
}

func cleanDirectory(path string) (string, error) {
	if !utf8.ValidString(path) || strings.ContainsRune(path, 0) || (!filepath.IsAbs(path) && !filepath.IsLocal(path)) {
		return "", tool.ErrInvalid
	}
	return filepath.Clean(path), nil
}

func validateExecutablePath(path string) error {
	if runtime.GOOS == "windows" && (strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) || (filepath.VolumeName(path) != "" && !filepath.IsAbs(path)) || strings.Contains(strings.TrimPrefix(path, filepath.VolumeName(path)), ":")) {
		return tool.ErrInvalid
	}
	return nil
}

func pinPath(path string, directory bool) (_ *fixedPath, retErr error) {
	actual, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, tool.ErrUnavailable
	}
	info, err := os.Stat(actual)
	if err != nil || (directory && !info.IsDir()) || (!directory && !info.Mode().IsRegular()) {
		return nil, tool.ErrUnavailable
	}
	file, err := openTargetPath(actual)
	if err != nil {
		return nil, tool.ErrUnavailable
	}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, file.Close())
		}
	}()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, tool.ErrDenied
	}
	identity, err := fileIdentity(file, opened)
	if err != nil {
		return nil, err
	}
	return &fixedPath{requested: path, actual: actual, file: file, info: opened, identity: identity}, nil
}

func (p *fixedPath) check(executable bool) error {
	actual, err := filepath.EvalSymlinks(p.requested)
	if err != nil || actual != p.actual {
		return tool.ErrDenied
	}
	current, err := os.Stat(actual)
	if err != nil || !os.SameFile(current, p.info) {
		return tool.ErrDenied
	}
	opened, err := p.file.Stat()
	if err != nil || !os.SameFile(opened, p.info) {
		return tool.ErrDenied
	}
	if executable && (opened.Size() != p.info.Size() || opened.Mode() != p.info.Mode() || !opened.ModTime().Equal(p.info.ModTime())) {
		return tool.ErrDenied
	}
	return nil
}

func prepare(ctx context.Context, scope tool.Scope, a arguments, environment []string, permissions tool.ExecutionPermissions, protection tool.ProtectedData) (_ tool.Invocation, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		return nil, tool.ErrUnavailable
	}
	fixed := &fixedCommand{command: a.Command, protection: protection, environment: slices.Clone(environment)}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, fixed.close())
		}
	}()
	if scope != (tool.Scope{}) {
		if scope.ID == "" || !filepath.IsAbs(scope.RootPath) {
			return nil, tool.ErrDenied
		}
		var err error
		fixed.workspace, err = pinPath(scope.RootPath, true)
		if err != nil {
			return nil, err
		}
	}
	cwd := a.Cwd
	if cwd == "" {
		cwd = scope.RootPath
	}
	if !filepath.IsAbs(cwd) {
		if scope.ID == "" {
			return nil, tool.ErrPathBase
		}
		cwd = filepath.Join(scope.RootPath, cwd)
	}
	if cwd == "" {
		return nil, tool.ErrPathBase
	}
	var err error
	fixed.directory, err = pinPath(cwd, true)
	if err != nil {
		return nil, err
	}
	program, err := resolveExecutable(a.Command[0], fixed.directory.actual, environment)
	if err != nil {
		return nil, err
	}
	fixed.executable, err = pinPath(program, false)
	if err != nil {
		return nil, err
	}
	fixed.command[0] = fixed.executable.actual
	fixed.environment = append(fixed.environment, "PWD="+fixed.directory.actual)
	if err := fixed.configureSandbox(a, permissions, protection); err != nil {
		return nil, err
	}
	digest := sha256.New()
	keys := make([]string, 0, len(fixed.environment))
	for _, entry := range fixed.environment {
		key, _, _ := strings.Cut(entry, "=")
		keys = append(keys, key)
		// 私有临时目录的随机名称不扩大操作范围，不能改变同一命令的拒绝/未知结果身份。
		if fixed.temporaryPath != "" {
			entry = strings.ReplaceAll(entry, fixed.temporaryPath, ":command_temp")
		}
		digest.Write([]byte(entry))
		digest.Write([]byte{0})
	}
	details := struct {
		Executable               string               `json:"executable"`
		ExecutableIdentity       string               `json:"executable_identity"`
		ExecutableSize           int64                `json:"executable_size"`
		ExecutableModified       int64                `json:"executable_modified_ns"`
		Cwd                      string               `json:"cwd"`
		CwdIdentity              string               `json:"cwd_identity"`
		EnvironmentProfile       string               `json:"environment_profile"`
		EnvironmentKeys          []string             `json:"environment_keys"`
		EnvironmentSHA256        string               `json:"environment_sha256"`
		Boundary                 tool.CommandBoundary `json:"execution_boundary"`
		Sandboxed                bool                 `json:"sandboxed"`
		SandboxBackend           string               `json:"sandbox_backend,omitempty"`
		WritableRoots            []string             `json:"writable_roots"`
		ReadableRoots            []string             `json:"readable_roots,omitempty"`
		TemporaryDirectoryPolicy string               `json:"temporary_directory_policy,omitempty"`
		NetworkAccess            bool                 `json:"network_access"`
		Justification            string               `json:"justification,omitempty"`
		ProfileSHA256            string               `json:"profile_sha256,omitempty"`
	}{Executable: fixed.executable.actual, ExecutableIdentity: fixed.executable.identity, ExecutableSize: fixed.executable.info.Size(), ExecutableModified: fixed.executable.info.ModTime().UnixNano(), Cwd: fixed.directory.actual, CwdIdentity: fixed.directory.identity, EnvironmentProfile: "base-v1", EnvironmentKeys: keys, EnvironmentSHA256: hex.EncodeToString(digest.Sum(nil)), Boundary: fixed.boundary, Sandboxed: fixed.boundary.Sandboxed, NetworkAccess: !fixed.boundary.Sandboxed, Justification: a.Justification, WritableRoots: []string{}}
	if fixed.boundary.Sandboxed {
		details.EnvironmentProfile = "base-v2-sandbox"
		details.SandboxBackend = fixed.sandboxBackend()
		details.ReadableRoots = fixed.sandboxState.readableRoots()
		details.TemporaryDirectoryPolicy = "private-per-command"
		if fixed.workspace != nil && fixed.boundary.Mode == tool.SandboxWorkspaceWrite {
			details.WritableRoots = append(details.WritableRoots, fixed.workspace.actual)
		}
		digest := sha256.Sum256([]byte(fixed.profile))
		details.ProfileSHA256 = hex.EncodeToString(digest[:])
	}
	fixed.snapshot, err = json.Marshal(details)
	if err != nil {
		return nil, err
	}
	if len(fixed.snapshot) > tool.MaxArgumentBytes {
		return nil, tool.ErrLimit
	}
	if err := fixed.check(ctx); err != nil {
		return nil, err
	}
	return &invocation{fixed: fixed}, nil
}

func (p *fixedCommand) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := p.sandboxState.check(ctx); err != nil {
		return err
	}
	if p.workspace != nil {
		if err := p.workspace.check(false); err != nil {
			return err
		}
	}
	for _, path := range []*fixedPath{p.sandboxExecutable, p.temporaryParent, p.protectedDirectory} {
		if path != nil {
			if err := path.check(path == p.sandboxExecutable); err != nil {
				return err
			}
		}
	}
	if p.boundary.Sandboxed {
		if err := checkSQLiteLinks(p.protection); err != nil {
			return err
		}
	}
	if err := p.directory.check(false); err != nil {
		return err
	}
	return p.executable.check(true)
}

func (p *fixedCommand) close() error {
	err := p.sandboxState.close()
	for _, path := range []*fixedPath{p.executable, p.directory, p.workspace, p.sandboxExecutable, p.temporaryParent, p.protectedDirectory} {
		if path != nil {
			err = errors.Join(err, path.file.Close())
		}
	}
	return err
}

func resolveExecutable(name, cwd string, environment []string) (string, error) {
	var candidates []string
	if filepath.IsAbs(name) {
		candidates = []string{name}
	} else if strings.ContainsAny(name, `/\`) {
		candidates = []string{filepath.Join(cwd, name)}
	} else {
		for _, directory := range filepath.SplitList(environmentValue(environment, "PATH")) {
			if filepath.IsAbs(directory) {
				candidates = append(candidates, filepath.Join(directory, name))
			}
		}
	}
	for _, candidate := range candidates {
		names := []string{candidate}
		if runtime.GOOS == "windows" && filepath.Ext(candidate) == "" {
			names = []string{candidate + ".exe", candidate + ".com"}
		}
		for _, path := range names {
			if runtime.GOOS == "windows" {
				ext := strings.ToLower(filepath.Ext(path))
				if ext != ".exe" && ext != ".com" {
					continue
				}
			}
			info, err := os.Stat(path)
			if err == nil && info.Mode().IsRegular() && (runtime.GOOS == "windows" || info.Mode().Perm()&0111 != 0) {
				return path, nil
			}
		}
	}
	return "", tool.ErrUnavailable
}
