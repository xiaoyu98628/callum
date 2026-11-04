package filesystem

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/xiaoyu98628/callum/internal/tool"
)

type patchVersion struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type patchArguments struct {
	Patch        string         `json:"patch"`
	BaseVersions []patchVersion `json:"base_versions"`
}

// patchTool 的互斥范围仅覆盖正在写入的重叠目标，不占用审批等待时间。
// 锁由一次 Tools 装配持有，无后台任务；执行结束立即删除活动项。
type patchTool struct {
	mu      sync.Mutex
	active  []*patchInvocation
	changed chan struct{}
}

func newPatchTool() *patchTool { return &patchTool{changed: make(chan struct{})} }
func (*patchTool) Definition() tool.Definition {
	return tool.Definition{Name: "apply_patch", Description: "创建或修改 UTF-8 文本文件，最多8个文件。patch 使用 *** Begin Patch / *** Add File: path（正文每行以+开始）/ *** Update File: path（@@ 后使用空格上下文、-旧行、+新行）/ *** End Patch。修改前用 read_file 获取整个文件的 sha256，在 base_versions 中为每个 Update File 提供同一路径和 sha256；新增文件不提供版本且不能覆盖已有文件。不支持删除、移动或模糊匹配。相对路径需要工作区，绝对路径不要求工作区。", Parameters: json.RawMessage(`{"type":"object","properties":{"patch":{"type":"string"},"base_versions":{"type":"array","items":{"type":"object","properties":{"path":{"type":"string"},"sha256":{"type":"string"}},"required":["path","sha256"],"additionalProperties":false}}},"required":["patch","base_versions"],"additionalProperties":false}`), Source: "builtin", Version: "1", Effects: []tool.Effect{tool.EffectWrite}}
}

func patchPath(path string) (string, error) {
	if path == "" || len(path) > 1024 || !utf8.ValidString(path) || strings.ContainsAny(path, "\x00\r\n") || (!filepath.IsAbs(path) && !filepath.IsLocal(path)) {
		return "", tool.ErrInvalid
	}
	if runtime.GOOS == "windows" {
		// 不接受设备命名空间、备用数据流或驱动器相对路径。
		if strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) || strings.Contains(strings.TrimPrefix(path, filepath.VolumeName(path)), ":") {
			return "", tool.ErrInvalid
		}
	}
	return filepath.Clean(path), nil
}

func decodePatch(raw json.RawMessage) (patchArguments, []patchFile, error) {
	var args patchArguments
	if len(raw) > tool.MaxArgumentBytes {
		return args, nil, tool.ErrLimit
	}
	if !utf8.Valid(raw) || !uniquePatchJSON(json.NewDecoder(bytes.NewReader(raw)), 0) {
		return args, nil, tool.ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&args) != nil || args.BaseVersions == nil {
		return args, nil, tool.ErrInvalid
	}
	var extra any
	if !errors.Is(d.Decode(&extra), io.EOF) {
		return args, nil, tool.ErrInvalid
	}
	files, err := parsePatch(args.Patch)
	if err != nil {
		return args, nil, err
	}
	versions := map[string]string{}
	for i, v := range args.BaseVersions {
		path, err := patchPath(v.Path)
		value, hashErr := hex.DecodeString(v.SHA256)
		if err != nil || hashErr != nil || len(value) != 32 || versions[path] != "" {
			return args, nil, tool.ErrInvalid
		}
		args.BaseVersions[i] = patchVersion{path, hex.EncodeToString(value)}
		versions[path] = args.BaseVersions[i].SHA256
	}
	for _, file := range files {
		if file.action == "update" {
			if versions[file.path] == "" {
				return args, nil, tool.ErrInvalid
			}
			delete(versions, file.path)
		}
	}
	if len(versions) != 0 {
		return args, nil, tool.ErrInvalid
	}
	slices.SortFunc(args.BaseVersions, func(a, b patchVersion) int { return strings.Compare(a.Path, b.Path) })
	args.Patch = canonicalPatch(files)
	return args, files, nil
}

// uniquePatchJSON 拒绝重复字段及过深结构，确保归一化和审批使用同一份含义。
func uniquePatchJSON(d *json.Decoder, depth int) bool {
	if depth > 8 {
		return false
	}
	token, err := d.Token()
	if err != nil {
		return false
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return true
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return false
			}
			s, ok := key.(string)
			if !ok || seen[s] {
				return false
			}
			seen[s] = true
			if !uniquePatchJSON(d, depth+1) {
				return false
			}
		}
	case '[':
		for d.More() {
			if !uniquePatchJSON(d, depth+1) {
				return false
			}
		}
	default:
		return false
	}
	_, err = d.Token()
	return err == nil
}
func (*patchTool) Validate(raw json.RawMessage) (json.RawMessage, error) {
	args, _, err := decodePatch(raw)
	if err != nil {
		return nil, err
	}
	normalized, err := json.Marshal(args)
	if len(normalized) > tool.MaxArgumentBytes {
		return nil, tool.ErrLimit
	}
	return normalized, err
}
func (t *patchTool) Prepare(ctx context.Context, scope tool.Scope, raw json.RawMessage) (_ tool.Invocation, retErr error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		return nil, tool.ErrUnavailable
	}
	args, files, err := decodePatch(raw)
	if err != nil {
		return nil, err
	}
	p := &patchInvocation{owner: t, scope: scope}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, p.Close())
		}
	}()
	if err := p.prepare(ctx, args, files); err != nil {
		return nil, err
	}
	return p, nil
}

func (t *patchTool) acquire(ctx context.Context, p *patchInvocation) (func(), error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		t.mu.Lock()
		busy := slices.ContainsFunc(t.active, p.overlaps)
		if !busy {
			t.active = append(t.active, p)
			t.mu.Unlock()
			return func() {
				t.mu.Lock()
				defer t.mu.Unlock()
				for i, active := range t.active {
					if active == p {
						t.active = append(t.active[:i], t.active[i+1:]...)
						break
					}
				}
				close(t.changed)
				t.changed = make(chan struct{})
			}, nil
		}
		changed := t.changed
		t.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-changed:
		}
	}
}
