// Package prepare 为开发和构建准备固定版本资源；服务运行期不调用本包。
package prepare

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/xiaoyu98628/callum/internal/runtimeassets"
)

// Source 固定归档的来源和校验值，Archive 是本地缓存文件名。
type Source struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Archive string `json:"archive"`
	Prefix  string `json:"prefix"`
	Entry   string `json:"entry"`
}

// GitSource 固定 Git 归档中相对于组件目录的配套路径。
type GitSource struct {
	Source
	ExecPath     string `json:"exec_path"`
	TemplatePath string `json:"template_path"`
	ConfigPath   string `json:"config_path"`
}

func (g GitSource) manifest() runtimeassets.GitRuntime {
	return runtimeassets.GitRuntime{Version: g.Version, Path: "git/" + g.Entry, ExecPath: "git/" + g.ExecPath, TemplatePath: "git/" + g.TemplatePath, ConfigPath: "git/" + g.ConfigPath}
}

// Target 将客户端的平台资源目录映射为 Go 平台和三个完整发行包。
type Target struct {
	Directory      string           `json:"directory"`
	Platform       string           `json:"platform"`
	Arch           string           `json:"arch"`
	Python         Source           `json:"python"`
	Node           Source           `json:"node"`
	Git            GitSource        `json:"git"`
	PythonPackages []PackageSource  `json:"python_packages,omitempty"`
	Workspace      *WorkspaceSource `json:"workspace,omitempty"`
}

// PackageSource 固定 PyPI wheel 的来源、校验值和模块入口；准备时只解压，不运行安装脚本。
type PackageSource struct {
	Name string `json:"name"`
	Source
}

type sources struct {
	FormatVersion int      `json:"format_version"`
	Targets       []Target `json:"targets"`
}

// ReadSources 接收固定本地归档及显式 HTTPS 来源，不执行归档内容或解析 latest。
// URL 为空的用户提供资源只允许使用已有本地归档。
func ReadSources(path string) (_ []Target, retErr error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, f.Close()) }()
	decoder := json.NewDecoder(io.LimitReader(f, 64<<10))
	decoder.DisallowUnknownFields()
	var value sources
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if decoder.Decode(new(any)) != io.EOF || value.FormatVersion != runtimeassets.FormatVersion || len(value.Targets) == 0 || len(value.Targets) > 16 {
		return nil, errors.New("资源来源清单无效")
	}
	seen := map[string]bool{}
	for _, target := range value.Targets {
		if !simpleName(target.Directory) || seen[target.Directory] || !slices.Contains([]string{"darwin", "windows", "linux"}, target.Platform) || !slices.Contains([]string{"arm64", "amd64"}, target.Arch) {
			return nil, errors.New("资源目标平台无效或重复")
		}
		for _, name := range []string{target.Git.ExecPath, target.Git.TemplatePath, target.Git.ConfigPath} {
			if !safePath(name) {
				return nil, errors.New("Git 配套路径无效")
			}
		}
		seen[target.Directory] = true
		if err := validatePackages(target); err != nil {
			return nil, err
		}
		if err := validateWorkspace(target.Workspace); err != nil {
			return nil, err
		}
		for _, source := range []Source{target.Python, target.Node, target.Git.Source} {
			parsed, err := url.Parse(source.URL)
			_, hashErr := hex.DecodeString(source.SHA256)
			if err != nil || (source.URL != "" && (parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "")) || hashErr != nil || len(source.SHA256) != 64 || source.Version == "" || !simpleName(source.Archive) || (!simpleName(source.Prefix) && source.Prefix != ".") || !safePath(source.Entry) {
				return nil, errors.New("运行时归档来源或入口无效")
			}
		}
	}
	return value.Targets, nil
}

func simpleName(name string) bool {
	return name != "." && name != ".." && name != "" && !strings.ContainsAny(name, `/\:`) && !strings.ContainsRune(name, 0)
}

// Install 在隔离暂存目录准备一个平台，并在全部校验成功后以 rename 激活。
// 已存在的目录只做一致性校验，拒绝覆盖损坏或不同版本资源；offline 禁止下载缺失归档。
func Install(ctx context.Context, target Target, outputRoot, cacheRoot string, offline bool) (retErr error) {
	if !filepath.IsAbs(outputRoot) || !filepath.IsAbs(cacheRoot) || !simpleName(target.Directory) {
		return errors.New("资源输出与缓存目录必须是绝对路径")
	}
	if err := os.MkdirAll(outputRoot, 0755); err != nil {
		return err
	}
	lock := filepath.Join(outputRoot, ".prepare-"+target.Directory+".lock")
	if err := os.Mkdir(lock, 0700); err != nil {
		return fmt.Errorf("资源准备锁已存在或不可创建: %w", err)
	}
	defer func() { retErr = errors.Join(retErr, os.Remove(lock)) }()
	archives := make(map[string]string, 3)
	packageArchives := make([]string, 0, len(target.PythonPackages))
	var workspaceArchives []string
	if err := validateWorkspace(target.Workspace); err != nil {
		return err
	}
	if target.Workspace != nil {
		for _, source := range target.Workspace.Archives {
			path := filepath.Join(cacheRoot, source.Archive)
			if err := verifyArchive(ctx, path, source.SHA256); err != nil {
				return fmt.Errorf("离线 工作区依赖归档 %s 不可用: %w", source.Archive, err)
			}
			workspaceArchives = append(workspaceArchives, path)
		}
	}
	for _, dependency := range target.PythonPackages {
		archive, err := cachedArchive(ctx, dependency.Source, cacheRoot, offline)
		if err != nil {
			return err
		}
		packageArchives = append(packageArchives, archive)
	}
	for _, item := range []struct {
		name   string
		source Source
	}{{"python", target.Python}, {"node", target.Node}, {"git", target.Git.Source}} {
		archive, err := cachedArchive(ctx, item.source, cacheRoot, offline)
		if err != nil {
			return err
		}
		archives[item.name] = archive
	}
	destination := filepath.Join(outputRoot, target.Directory)
	if _, err := os.Lstat(destination); err == nil {
		file, err := os.Open(filepath.Join(destination, "manifest.json"))
		if err != nil {
			return errors.New("已有运行时目录缺少清单，拒绝覆盖")
		}
		manifest, decodeErr := runtimeassets.Decode(file)
		if err := errors.Join(decodeErr, file.Close()); err != nil {
			return err
		}
		if !slices.Equal(manifest.PythonPackages, packageManifest(target)) {
			return errors.New("已有运行时的 Python 依赖与清单不一致，请在独立输出目录重新准备")
		}
		if !sameWorkspace(manifest.Workspace, target.Workspace) {
			return errors.New("已有运行时的完整 工作区依赖资源不一致，请在独立目录重新准备")
		}
		files, err := runtimeassets.Inventory(ctx, destination)
		if err != nil || manifest.Platform != target.Platform || manifest.Arch != target.Arch || manifest.Python != (runtimeassets.Executable{Version: target.Python.Version, Path: "python/" + target.Python.Entry}) || manifest.Node != (runtimeassets.Executable{Version: target.Node.Version, Path: "node/" + target.Node.Entry}) || manifest.Git != target.Git.manifest() || !slices.Equal(files, manifest.Files) {
			return errors.New("已有运行时资源与固定版本不一致，拒绝覆盖")
		}
		return runtimeassets.ValidateLayout(destination, manifest)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	stage, err := os.MkdirTemp(outputRoot, ".prepare-"+target.Directory+"-")
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, removeStage(stage)) }()
	for _, item := range []struct {
		name   string
		source Source
	}{{"python", target.Python}, {"node", target.Node}, {"git", target.Git.Source}} {
		component := filepath.Join(stage, item.name)
		if err := extract(ctx, archives[item.name], component, item.source.Prefix); err != nil {
			return fmt.Errorf("解压 %s: %w", item.name, err)
		}
	}
	if target.Platform == "windows" {
		// Windows 不依赖符号链接权限；提供与 Unix 一致的 python3 命令名称。
		original := filepath.Join(stage, "python", filepath.FromSlash(target.Python.Entry))
		alias := filepath.Join(filepath.Dir(original), "python3.exe")
		if _, err := os.Stat(alias); errors.Is(err, os.ErrNotExist) {
			data, err := os.ReadFile(original)
			if err != nil {
				return err
			}
			if err := os.WriteFile(alias, data, 0755); err != nil {
				return err
			}
		}
	}
	if err := installPackages(ctx, target, stage, packageArchives); err != nil {
		return err
	}
	if len(workspaceArchives) > 0 {
		for _, archive := range workspaceArchives {
			if err := extractInto(ctx, archive, stage, "."); err != nil {
				return fmt.Errorf("解压工作区依赖: %w", err)
			}
		}
	}
	files, err := runtimeassets.Inventory(ctx, stage)
	if err != nil {
		return err
	}
	manifest := runtimeassets.Manifest{FormatVersion: runtimeassets.FormatVersion, Platform: target.Platform, Arch: target.Arch, Python: runtimeassets.Executable{Version: target.Python.Version, Path: "python/" + target.Python.Entry}, Node: runtimeassets.Executable{Version: target.Node.Version, Path: "node/" + target.Node.Entry}, Git: target.Git.manifest(), PythonPackages: packageManifest(target), Files: files}
	if target.Workspace != nil {
		manifest.Workspace = new(target.Workspace.Runtime)
	}
	if err := runtimeassets.ValidateLayout(stage, manifest); err != nil {
		return err
	}
	if err := runtimeassets.WriteManifest(stage, manifest); err != nil {
		return err
	}
	if err := filepath.WalkDir(stage, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		mode := fs.FileMode(0444)
		if info.IsDir() || info.Mode().Perm()&0111 != 0 {
			mode = 0555
		}
		return os.Chmod(path, mode)
	}); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Rename(stage, destination)
}

func removeStage(directory string) error {
	if _, err := os.Stat(directory); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.Chmod(path, 0700)
		}
		return nil
	}); err != nil {
		return err
	}
	return os.RemoveAll(directory)
}

func cachedArchive(ctx context.Context, source Source, cache string, offline bool) (_ string, retErr error) {
	if !offline {
		if err := os.MkdirAll(cache, 0700); err != nil {
			return "", err
		}
	}
	path := filepath.Join(cache, source.Archive)
	if _, err := os.Stat(path); err == nil {
		return path, verifyArchive(ctx, path, source.SHA256)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if offline {
		return "", fmt.Errorf("缺少本地运行时归档 %s，离线模式不允许下载", source.Archive)
	}
	if source.URL == "" {
		return "", fmt.Errorf("归档 %s 仅支持已核验的本地资源", source.Archive)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return "", err
	}
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment, ForceAttemptHTTP2: true, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second, IdleConnTimeout: 30 * time.Second}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 10 * time.Minute, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if request.URL.Scheme != "https" || len(via) >= 10 {
			return errors.New("运行时下载重定向无效")
		}
		return nil
	}}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer func() { retErr = errors.Join(retErr, response.Body.Close()) }()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("运行时下载返回 HTTP %d", response.StatusCode)
	}
	file, err := os.CreateTemp(cache, ".download-")
	if err != nil {
		return "", err
	}
	defer func() {
		if err := os.Remove(file.Name()); err != nil && !errors.Is(err, os.ErrNotExist) {
			retErr = errors.Join(retErr, err)
		}
	}()
	n, copyErr := io.Copy(file, io.LimitReader(response.Body, (512<<20)+1))
	if err := errors.Join(copyErr, file.Close()); err != nil {
		return "", err
	}
	if n > 512<<20 {
		return "", errors.New("运行时归档超过大小限制")
	}
	if err := verifyArchive(ctx, file.Name(), source.SHA256); err != nil {
		return "", err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return "", err
	}
	return path, nil
}

func verifyArchive(ctx context.Context, path, expected string) (retErr error) {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, file.Close()) }()
	hash := sha256.New()
	buffer := make([]byte, 128<<10)
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := file.Read(buffer)
		size += int64(n)
		if size > 512<<20 {
			return errors.New("运行时归档超过大小限制")
		}
		_, _ = hash.Write(buffer[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected {
		return errors.New("运行时归档 SHA-256 不匹配")
	}
	return nil
}
