package tool

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ProtectedData 固定服务端自己的数据目录和 SQLite 文件位置。空值用于没有应用存储的调用方。
type ProtectedData struct {
	Directory           string
	Database            string
	ReadOnlyDirectories []string // 装配层传入已解析链接的实际绝对路径。
}

// WriteProtected 拒绝修改私有数据或只读运行时；读取运行时仍按普通文件规则处理。
func (p ProtectedData) WriteProtected(path string) bool {
	return p.Contains(path) || p.ReadOnlyContains(path)
}

// ReadProtected 仅向装配层登记的只读子树开放私有数据读取；写保护和 SQLite 身份检查保持独立。
func (p ProtectedData) ReadProtected(path string) bool {
	if !p.Contains(path) {
		return false
	}
	for _, root := range p.ReadOnlyDirectories {
		resource := ProtectedData{Directory: root}
		if p.Contains(root) && !resource.Contains(p.Directory) && resource.Contains(path) {
			return false
		}
	}
	return true
}

// ReadOnlyContains 判断实际目标是否属于只读资源树，不限制读取。
func (p ProtectedData) ReadOnlyContains(path string) bool {
	for _, root := range p.ReadOnlyDirectories {
		if (ProtectedData{Directory: root}).Contains(path) {
			return true
		}
	}
	return false
}

// ProtectsDescendant 也保护资源目录的祖先，防止通过重命名祖先改变固定路径。
func (p ProtectedData) ProtectsDescendant(path string) bool {
	for _, root := range append([]string{p.Directory}, p.ReadOnlyDirectories...) {
		if root != "" && (ProtectedData{Directory: path}).Contains(root) {
			return true
		}
	}
	return false
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
