// Package runtimeassets 校验随应用提供的 Python、Node 和 Git 资源，不下载资源或修改全局环境。
package runtimeassets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// FormatVersion 固定当前完整资源清单结构；旧缓存由调用方使用独立目录保留。
const FormatVersion = 3

const MaxManifestBytes = 8 << 20
const maxFiles = 30000
const maxPayloadBytes int64 = 2 << 30

// Executable 记录资源内入口；Path 是使用斜杠分隔的相对路径。
type Executable struct {
	Version string `json:"version"`
	Path    string `json:"path"`
}

// PythonPackage 记录离线组合进统一解释器的第三方依赖及其模块入口。
type PythonPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Path    string `json:"path"`
}

// WorkspaceRuntime 固定技能共享依赖，解释器复用 Manifest 中的统一入口。
type WorkspaceRuntime struct {
	Version      string `json:"version"`
	NodeModules  string `json:"node_modules"`
	BinDirectory string `json:"bin_directory"`
}

// GitRuntime 固定 Git 入口及其发行包内的辅助程序、模板和系统配置路径。
type GitRuntime struct {
	Executable
	ExecPath     string `json:"exec_path"`
	TemplatePath string `json:"template_path"`
	ConfigPath   string `json:"config_path"`
}

// File 记录普通文件的内容，或符号链接原文；不允许链接越出资源根目录。
type File struct {
	Path   string `json:"path"`
	Size   int64  `json:"size,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	Link   string `json:"link,omitempty"`
}

// Manifest 固定一个平台的完整运行时资源，准备命令生成后由应用只读使用。
type Manifest struct {
	FormatVersion  int               `json:"format_version"`
	Platform       string            `json:"platform"`
	Arch           string            `json:"arch"`
	Python         Executable        `json:"python"`
	Node           Executable        `json:"node"`
	Git            GitRuntime        `json:"git"`
	PythonPackages []PythonPackage   `json:"python_packages,omitempty"`
	Workspace      *WorkspaceRuntime `json:"workspace,omitempty"`
	Files          []File            `json:"files"`
}

func localPath(name string) bool {
	return name != "." && fs.ValidPath(name) && !strings.ContainsAny(name, `\:`) && !strings.ContainsRune(name, 0)
}

// Decode 严格限制清单大小、字段和尾随内容；具体文件与平台由 Load 复核。
func Decode(reader io.Reader) (Manifest, error) {
	var value Manifest
	data, err := io.ReadAll(io.LimitReader(reader, MaxManifestBytes+1))
	if err != nil || len(data) > MaxManifestBytes {
		return value, errors.New("运行时清单读取失败或超过限制")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, fmt.Errorf("运行时清单无效: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF || value.FormatVersion != FormatVersion || len(value.Files) == 0 || len(value.Files) > maxFiles {
		return value, errors.New("运行时清单格式或文件数量无效")
	}
	return value, nil
}

func hashFile(ctx context.Context, root *os.Root, name string) (_ string, size int64, retErr error) {
	f, err := root.Open(filepath.FromSlash(name))
	if err != nil {
		return "", 0, err
	}
	defer func() { retErr = errors.Join(retErr, f.Close()) }()
	hash := sha256.New()
	buffer := make([]byte, 128<<10)
	for {
		if err := ctx.Err(); err != nil {
			return "", size, err
		}
		n, err := f.Read(buffer)
		if n > 0 {
			size += int64(n)
			if size > maxPayloadBytes {
				return "", size, errors.New("运行时资源超过大小限制")
			}
			_, _ = hash.Write(buffer[:n]) // hash.Hash.Write 不返回错误。
		}
		if err == io.EOF {
			return hex.EncodeToString(hash.Sum(nil)), size, nil
		}
		if err != nil {
			return "", size, err
		}
	}
}

// Inventory 计算已准备目录的完整文件清单，不包含根目录的 manifest.json。
// 调用方必须保证准备目录不与任务执行共享，返回前不激活该目录。
func Inventory(ctx context.Context, directory string) (_ []File, retErr error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()
	var files []File
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || name == "manifest.json" {
			return nil
		}
		if !localPath(name) || len(files) >= maxFiles {
			return errors.New("运行时文件路径或数量无效")
		}
		item := File{Path: name}
		if entry.Type()&os.ModeSymlink != 0 {
			item.Link, err = root.Readlink(filepath.FromSlash(name))
			if err != nil || filepath.IsAbs(item.Link) || !localPath(path.Join(path.Dir(name), filepath.ToSlash(item.Link))) {
				return errors.New("运行时链接越出资源目录")
			}
			// OpenRoot 校验完整链接链，包括中间目录链接。
			if _, err := root.Stat(filepath.FromSlash(name)); err != nil {
				return fmt.Errorf("运行时链接不可用: %w", err)
			}
		} else if entry.Type().IsRegular() {
			// 目录顺序先固定，再由有界工作者计算内容摘要，避免完整依赖树的
			// 小文件安全打开与大型库读取串行占满启动时限。
		} else {
			return errors.New("运行时资源包含特殊文件")
		}
		files = append(files, item)
		return nil
	})
	if err != nil {
		return nil, err
	}
	checks, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	var next, total atomic.Int64
	var workers sync.WaitGroup
	for range min(4, len(files)) {
		workers.Go(func() {
			for checks.Err() == nil {
				index := next.Add(1) - 1
				if index >= int64(len(files)) {
					return
				}
				item := &files[index]
				if item.Link != "" {
					continue
				}
				var err error
				item.SHA256, item.Size, err = hashFile(checks, root, item.Path)
				if err != nil {
					cancel(err)
					return
				}
				if total.Add(item.Size) > maxPayloadBytes {
					cancel(errors.New("运行时资源总大小超过限制"))
					return
				}
			}
		})
	}
	// 返回和关闭 Root 前等待所有校验工作者；失败会取消其他读取，不留下后台任务。
	workers.Wait()
	if err := context.Cause(checks); err != nil {
		return nil, err
	}
	return files, nil
}
