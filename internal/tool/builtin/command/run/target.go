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
	command                          []string
	environment                      []string
	executable, directory, workspace *fixedPath
	snapshot                         json.RawMessage
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

func prepare(ctx context.Context, scope tool.Scope, a arguments, environment []string) (_ tool.Invocation, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		return nil, tool.ErrUnavailable
	}
	fixed := &fixedCommand{command: a.Command, environment: environment}
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
	digest := sha256.New()
	keys := make([]string, 0, len(fixed.environment))
	for _, entry := range fixed.environment {
		key, _, _ := strings.Cut(entry, "=")
		keys = append(keys, key)
		digest.Write([]byte(entry))
		digest.Write([]byte{0})
	}
	details := struct {
		Executable         string   `json:"executable"`
		ExecutableIdentity string   `json:"executable_identity"`
		ExecutableSize     int64    `json:"executable_size"`
		ExecutableModified int64    `json:"executable_modified_ns"`
		Cwd                string   `json:"cwd"`
		CwdIdentity        string   `json:"cwd_identity"`
		EnvironmentProfile string   `json:"environment_profile"`
		EnvironmentKeys    []string `json:"environment_keys"`
		EnvironmentSHA256  string   `json:"environment_sha256"`
		Sandboxed          bool     `json:"sandboxed"`
	}{fixed.executable.actual, fixed.executable.identity, fixed.executable.info.Size(), fixed.executable.info.ModTime().UnixNano(), fixed.directory.actual, fixed.directory.identity, "base-v1", keys, hex.EncodeToString(digest.Sum(nil)), false}
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
	if p.workspace != nil {
		if err := p.workspace.check(false); err != nil {
			return err
		}
	}
	if err := p.directory.check(false); err != nil {
		return err
	}
	return p.executable.check(true)
}

func (p *fixedCommand) close() error {
	var err error
	for _, path := range []*fixedPath{p.executable, p.directory, p.workspace} {
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
