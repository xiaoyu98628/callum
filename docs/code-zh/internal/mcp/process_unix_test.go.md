```go
//go:build darwin || linux

package MCP

import (
	进程执行包 "os/exec"
	测试包 "testing"
)

func Test已退出MCP进程组可被已关闭(测试器 *测试包.测试器) {
	for range 20 {
		完成 := make(chan struct{})
		命令 := 进程执行包.命令上下文(测试器.上下文(), "/usr/bin/true")
		停止, 错误 := 开始进程(命令, 完成)
		if 错误 != nil {
			测试器.报告致命错误(错误)
		}
		if 错误 := 命令.等待(); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		close(完成)
		if 错误 := 停止(); 错误 != nil {
			测试器.格式化报告致命错误("退出后无法清理进程组: %v", 错误)
		}
	}
}
```
