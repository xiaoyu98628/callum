package sqlite

import (
	"errors"
	"os"
	"sync"
)

// Ownership 持有服务实例的文件锁；Close 在运行任务结束后释放，可重复调用。
type Ownership struct {
	file *os.File
	once sync.Once
	err  error
}

// AcquireOwnership 防止另一个服务实例在同一数据目录执行恢复或写入运行状态。
// SQLite 普通连接仍可用于配置查询；app 必须在 Recover 前取得并持有此锁。
func AcquireOwnership(databasePath string) (*Ownership, error) {
	path := databasePath + ".lock"
	if err := prepareFile(path); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockOwnership(file); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return &Ownership{file: file}, nil
}

func (o *Ownership) Close() error { o.once.Do(func() { o.err = o.file.Close() }); return o.err }
