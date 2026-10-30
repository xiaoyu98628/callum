package database

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func prepareFile(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建数据目录: %w", err)
	}
	if err := checkPrivate(dir, true); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err == nil {
		return f.Close()
	}
	if !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("创建数据库文件: %w", err)
	}
	return checkPrivate(path, false)
}

func checkPrivate(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("检查数据路径: %w", err)
	}
	if directory && !info.IsDir() || !directory && !info.Mode().IsRegular() {
		return errors.New("数据路径类型无效，不接受符号链接")
	}
	// Windows 需要由部署环境配置 ACL，不能通过 Unix 权限位推断。
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return errors.New("已有数据路径权限过宽，需要用户调整访问权限")
	}
	return nil
}
