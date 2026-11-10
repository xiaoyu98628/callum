package run

import (
	"syscall"

	"github.com/xiaoyu98628/callum/internal/tool"
)

// Seatbelt 按路径判断，已存在的硬链接可从另一个路径访问同一文件。
// 存在此类别名时拒绝启动受管命令，不能把数据库可读误判为已隔离。
func checkSQLiteLinks(protection tool.ProtectedData) error {
	files, err := protection.SQLiteFiles()
	if err != nil {
		return err
	}
	for _, file := range files {
		stat, ok := file.Sys().(*syscall.Stat_t)
		if !ok || stat.Nlink != 1 {
			return tool.ErrDenied
		}
	}
	return nil
}
