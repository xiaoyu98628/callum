package fileaccess

import (
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/xiaoyu98628/callum/internal/tool"
)

// CleanPath 校验文件工具的路径格式，不解析目标或决定访问权限。
func CleanPath(path string) (string, error) {
	if path == "" || !utf8.ValidString(path) || (!filepath.IsAbs(path) && !filepath.IsLocal(path)) || strings.ContainsRune(path, 0) {
		return "", tool.ErrInvalid
	}
	return filepath.Clean(path), nil
}
