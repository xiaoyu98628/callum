//go:build !darwin

package run

import "github.com/xiaoyu98628/callum/internal/tool"

// 其他平台的命令当前按沙箱外边界审批。
func checkSQLiteLinks(tool.ProtectedData) error { return nil }
