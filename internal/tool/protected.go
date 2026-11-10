package tool

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ProtectedData 固定服务端自己的数据目录和 SQLite 文件位置。空值用于没有应用存储的调用方。
type ProtectedData struct {
	Directory string
	Database  string
}

// Contains 判断已解析的目标是否在数据目录内，包括大小写不敏感卷上的路径变体。
func (p ProtectedData) Contains(path string) bool {
	if p.Directory == "" {
		return false
	}
	relative, err := filepath.Rel(strings.ToLower(p.Directory), strings.ToLower(path))
	return err == nil && filepath.IsLocal(relative)
}

// SQLiteFiles 检查数据库与附属文件的当前身份；附属文件可随连接活动创建或移除。
func (p ProtectedData) SQLiteFiles() ([]os.FileInfo, error) {
	if p.Database == "" {
		return nil, nil
	}
	var files []os.FileInfo
	for _, suffix := range []string{"", "-journal", "-wal", "-shm", ".lock"} {
		info, err := os.Stat(p.Database + suffix)
		if suffix != "" && errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			return nil, ErrDenied
		}
		files = append(files, info)
	}
	return files, nil
}

// CheckFile 拒绝使用其他路径访问受保护的 SQLite 文件身份。
func (p ProtectedData) CheckFile(info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return nil
	}
	files, err := p.SQLiteFiles()
	if err != nil {
		return err
	}
	for _, file := range files {
		if os.SameFile(info, file) {
			return ErrDenied
		}
	}
	return nil
}
