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
	if !localPath(manifest.Python.Path) || !strings.HasPrefix(manifest.Python.Path, "python/") || !localPath(manifest.Node.Path) || !strings.HasPrefix(manifest.Node.Path, "node/") || manifest.Python.Version == "" || manifest.Node.Version == "" {
		return nil, errors.New("内置解释器入口无效")
	}
	files, err := Inventory(ctx, actual)
	if err != nil {
		return nil, err
	}
	if !slices.Equal(files, manifest.Files) {
		return nil, errors.New("内置运行时资源缺失、损坏或存在清单外文件，请重新准备资源")
	}
	for _, name := range []string{manifest.Python.Path, manifest.Node.Path} {
		info, err := root.Stat(filepath.FromSlash(name))
		if err != nil || !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0) {
			return nil, errors.New("内置解释器不存在或不可执行")
		}
	}
	info, err := os.Stat(actual)
	if err != nil {
		return nil, err
	}
	return &Bundle{directory: actual, manifest: manifest, identity: info, python: filepath.Join(actual, filepath.FromSlash(manifest.Python.Path)), node: filepath.Join(actual, filepath.FromSlash(manifest.Node.Path))}, nil
}

// Directory 返回已解析符号链接的实际资源根目录。
func (b *Bundle) Directory() string { return b.directory }

// BinDirectories 返回仅供命令子进程使用的搜索目录，不修改系统或服务进程 PATH。
func (b *Bundle) BinDirectories() []string {
	return []string{filepath.Dir(b.python), filepath.Dir(b.node)}
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
	default:
		return "", false
	}
}

// Check 在每次准备操作时复核目录和两个解释器；标准库全量校验在启动时完成。
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
	for _, executable := range []Executable{b.manifest.Python, b.manifest.Node} {
		name := executable.Path
		for {
			index := slices.IndexFunc(b.manifest.Files, func(file File) bool { return file.Path == name })
			if index < 0 {
				return errors.New("内置解释器不在清单中")
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
					return errors.New("内置解释器链接无效")
				}
				name = filepath.ToSlash(rel)
				continue
			}
			hash, size, err := hashFile(ctx, root, name)
			if err != nil {
				return fmt.Errorf("内置解释器完整性检查失败: %w", err)
			}
			if hash != file.SHA256 || size != file.Size {
				return errors.New("内置解释器完整性检查失败")
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
