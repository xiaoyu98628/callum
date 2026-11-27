```go
//go:build darwin || linux

package MCP

import (
	进程执行包 "os/exec"
	系统调用包 "syscall"

	进程组包 "github.com/xiaoyu98628/callum/internal/processgroup"
)

func 开始进程(命令 *进程执行包.命令, 完成 <-chan struct{}) (func() error, error) {
	命令.系统进程属性 = &系统调用包.系统进程属性{设置进程组: true}
	if 错误 := 命令.启动(); 错误 != nil {
		return nil, 错误
	}
	进程标识 := 命令.进程.进程标识
	return func() error {
		已退出 := false
		select {
		case <-完成:
			已退出 = true
		default:
		}
		return 进程组包.停止(进程标识, 已退出, 进程停止等待)
	}, nil
}
```
