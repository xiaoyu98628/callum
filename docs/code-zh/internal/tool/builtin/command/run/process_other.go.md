```go
//go:build !darwin && !linux && !windows

package 运行

import (
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	操作系统包 "os"
)

type 进程 struct{}

func (*进程) 获取进程标识() int { return 0 }

func 打开目标路径(string) (*操作系统包.文件, error) { return nil, 工具包.不可用错误 }

func 开始进程(*固定命令, *操作系统包.文件, *操作系统包.文件, *操作系统包.文件) (*进程, error) {
	return nil, 工具包.不可用错误
}
func (*进程) 轮询() (进程退出, bool, error)        { return 进程退出{}, false, 工具包.不可用错误 }
func (*进程) 停止() error                             { return 工具包.不可用错误 }
func (*进程) close() error                            { return nil }
func 文件身份(*操作系统包.文件, 操作系统包.文件信息) (string, error) { return "", 工具包.不可用错误 }
```
