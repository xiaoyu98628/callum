```go
//go:build !darwin && !linux && !windows

package MCP

import 进程执行包 "os/exec"

func 开始进程(*进程执行包.命令, <-chan struct{}) (func() error, error) { return nil, 连接错误 }
```
