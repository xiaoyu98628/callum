package runtimeassets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// Bundle 是启动时校验的只读资源描述；不持有文件或后台任务。
// 同账户的外部修改不构成隔离边界；命令执行器另固定句柄并复核入口。
type Bundle struct {
	directory string
	manifest  Manifest
	identity  os.FileInfo
	python    string
	node      string
	git       string
}

// Load 校验平台、所有文件内容和链接，返回可供装配层借用的固定描述。
func Load(ctx context.Context, directory string) (_ *Bundle, retErr error) {
	if !filepath.IsAbs(directory) {
		return nil, errors.New("运行时目录必须是绝对路径")
	}
	actual, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return nil, fmt.Errorf("定位内置运行时: %w", err)
	}
	root, err := os.OpenRoot(actual)
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()
	file, err := root.Open("manifest.json")
	if err != nil {
		return nil, fmt.Errorf("读取内置运行时清单: %w", err)
	}
	manifest, decodeErr := Decode(file)
	if err := errors.Join(decodeErr, file.Close()); err != nil {
		return nil, err
	}
	if manifest.Platform != runtime.GOOS || manifest.Arch != runtime.GOARCH {
		return nil, errors.New("内置运行时与当前平台或架构不匹配")
	}
	if err := ValidateLayout(actual, manifest); err != nil {
		return nil, err
	}
	files, err := Inventory(ctx, actual)
	if err != nil {
		return nil, err
	}
	if !slices.Equal(files, manifest.Files) {
		return nil, errors.New("内置运行时资源缺失、损坏或存在清单外文件，请重新准备资源")
	}
	info, err := os.Stat(actual)
	if err != nil {
		return nil, err
	}
	return &Bundle{directory: actual, manifest: manifest, identity: info, python: filepath.Join(actual, filepath.FromSlash(manifest.Python.Path)), node: filepath.Join(actual, filepath.FromSlash(manifest.Node.Path)), git: filepath.Join(actual, filepath.FromSlash(manifest.Git.Path))}, nil
}

// Directory 返回已解析符号链接的实际资源根目录。
func (b *Bundle) Directory() string { return b.directory }

// BinDirectories 返回仅供命令子进程使用的搜索目录，不修改系统或服务进程 PATH。
func (b *Bundle) BinDirectories() []string {
	directories := []string{filepath.Dir(b.python), filepath.Dir(b.node), filepath.Dir(b.git), filepath.Join(b.directory, filepath.FromSlash(b.manifest.Git.ExecPath))}
	if b.manifest.Platform == "windows" {
		// Windows 的 DLL、HTTPS/GCM 和 MSYS shell/ssh 分布在两个附属 bin 目录。
		prefix := filepath.Dir(filepath.Dir(filepath.Join(b.directory, filepath.FromSlash(b.manifest.Git.ExecPath))))
		directories = append(directories, filepath.Join(prefix, "bin"), filepath.Join(b.directory, "git", "usr", "bin"))
	}
	return directories
}

// Resolve 仅映射裸命令名；显式路径继续使用现有路径校验与审批规则。
func (b *Bundle) Resolve(name string) (string, bool) {
	if b == nil {
		return "", false
	}
	if runtime.GOOS == "windows" {
		name = strings.TrimSuffix(strings.ToLower(name), ".exe")
	}
	switch name {
	case "python", "python3":
		return b.python, true
	case "node":
		return b.node, true
	case "git":
		return b.git, true
	default:
		return "", false
	}
}

// Check 在每次准备操作时复核目录、三个命令入口及 Git 配置；完整资源校验在启动时完成。
func (b *Bundle) Check(ctx context.Context) (retErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	actual, err := filepath.EvalSymlinks(b.directory)
	if err != nil || actual != b.directory {
		return errors.New("内置运行时目录已变化")
	}
	info, err := os.Stat(actual)
	if err != nil || !os.SameFile(info, b.identity) {
		return errors.New("内置运行时目录身份已变化")
	}
	root, err := os.OpenRoot(actual)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()
	for _, executable := range []Executable{b.manifest.Python, b.manifest.Node, b.manifest.Git.Executable, {Path: b.manifest.Git.ConfigPath}} {
		name := executable.Path
		for {
			index := slices.IndexFunc(b.manifest.Files, func(file File) bool { return file.Path == name })
			if index < 0 {
				return errors.New("内置命令不在清单中")
			}
			file := b.manifest.Files[index]
			if file.Link != "" {
				// Inventory 已验证链接链；此处解析实际目标并按清单校验内容。
				resolved, err := filepath.EvalSymlinks(filepath.Join(actual, filepath.FromSlash(name)))
				if err != nil {
					return err
				}
				rel, err := filepath.Rel(actual, resolved)
				if err != nil || !filepath.IsLocal(rel) || filepath.ToSlash(rel) == name {
					return errors.New("内置命令链接无效")
				}
				name = filepath.ToSlash(rel)
				continue
			}
			hash, size, err := hashFile(ctx, root, name)
			if err != nil {
				return fmt.Errorf("内置命令完整性检查失败: %w", err)
			}
			if hash != file.SHA256 || size != file.Size {
				return errors.New("内置命令完整性检查失败")
			}
			break
		}
	}
	return nil
}

// WriteManifest 只供离线资源准备和测试使用；运行期不写入资源目录。
func WriteManifest(directory string, manifest Manifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil || len(data) > MaxManifestBytes {
		return errors.New("无法生成运行时清单")
	}
	return os.WriteFile(filepath.Join(directory, "manifest.json"), append(data, '\n'), 0444)
}

// ValidateLayout 检查清单入口及 Git 配套目录，供准备阶段和运行期启动共用。
func ValidateLayout(directory string, manifest Manifest) error {
	for component, executable := range map[string]Executable{"python": manifest.Python, "node": manifest.Node, "git": manifest.Git.Executable} {
		if executable.Version == "" || !localPath(executable.Path) || !strings.HasPrefix(executable.Path, component+"/") {
			return errors.New("内置命令入口无效")
		}
		info, err := os.Stat(filepath.Join(directory, filepath.FromSlash(executable.Path)))
		if err != nil || !info.Mode().IsRegular() || (manifest.Platform != "windows" && info.Mode().Perm()&0111 == 0) {
			return errors.New("内置命令不存在或不可执行")
		}
	}
	for _, resource := range []struct {
		name        string
		isDirectory bool
	}{{manifest.Git.ExecPath, true}, {manifest.Git.TemplatePath, true}, {manifest.Git.ConfigPath, false}} {
		name, isDirectory := resource.name, resource.isDirectory
		if !localPath(name) || !strings.HasPrefix(name, "git/") {
			return errors.New("内置 Git 辅助资源路径无效")
		}
		info, err := os.Stat(filepath.Join(directory, filepath.FromSlash(name)))
		if err != nil || info.IsDir() != isDirectory || (!isDirectory && !info.Mode().IsRegular()) {
			return errors.New("内置 Git 辅助资源缺失或类型无效")
		}
	}
	return nil
}

// GitEnvironment 仅供命令子进程使用；不写入用户配置，不启用 GUI 或终端交互。
func (b *Bundle) GitEnvironment() []string {
	absolute := func(name string) string { return filepath.Join(b.directory, filepath.FromSlash(name)) }
	return []string{"GIT_EXEC_PATH=" + absolute(b.manifest.Git.ExecPath), "GIT_TEMPLATE_DIR=" + absolute(b.manifest.Git.TemplatePath), "GIT_CONFIG_SYSTEM=" + absolute(b.manifest.Git.ConfigPath), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=0", "GIT_ASKPASS=", "SSH_ASKPASS=", "GIT_PAGER=cat"}
}
