package tool

import "path/filepath"

// DirectoryScope 是用户可明确授予的真实目录范围，读写授权包含读取。
// 它授权路径范围；每次操作仍须固定并复核实际目标，不授权命令或其他影响。
type DirectoryScope struct {
	RootPath string `json:"root_path"`
	Access   string `json:"access"`
}

// Valid 检查服务端规范化的绝对路径与受支持的授权类别。
func (s DirectoryScope) Valid() bool {
	return filepath.IsAbs(s.RootPath) && filepath.Clean(s.RootPath) == s.RootPath && (s.Access == "read" || s.Access == "read_write")
}
