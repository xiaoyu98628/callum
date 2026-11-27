```go
package main

import (
	上下文包 "context"
	错误判断包 "errors"
	命令行参数包 "flag"
	结构化日志包 "log/slog"
	操作系统包 "os"
	系统信号包 "os/signal"
	系统调用包 "syscall"

	应用模块包 "github.com/xiaoyu98628/callum/internal/app"
	配置模块包 "github.com/xiaoyu98628/callum/internal/config"
	准备模块包 "github.com/xiaoyu98628/callum/internal/runtimeassets/prepare"
	传输模块包 "github.com/xiaoyu98628/callum/internal/stdio/transport"
)

func main() {
	操作系统包.退出进程(运行())
}

func 运行() int {
	日志记录器 := 结构化日志包.新建(结构化日志包.新建JSON处理器(操作系统包.标准错误输出, nil))
	if len(操作系统包.启动参数) > 1 && 操作系统包.启动参数[1] == "resource-lock" {
		return 运行资源锁(操作系统包.启动参数[2:], 日志记录器)
	}
	if len(操作系统包.启动参数) > 1 && 操作系统包.启动参数[1] == "prepare-runtimes" {
		执行上下文, 停止 := 系统信号包.创建信号上下文(上下文包.后台上下文(), 操作系统包.中断信号, 系统调用包.终止信号)
		defer 停止()
		if 错误 := 准备模块包.运行命令(执行上下文, 操作系统包.启动参数[2:], 日志记录器.附加字段("module", "runtimeassets.prepare"), true); 错误 != nil && !错误判断包.是否属于(错误, 命令行参数包.请求帮助错误) {
			日志记录器.记录错误("离线运行时准备失败", "module", "runtimeassets.prepare", "error", 错误)
			return 1
		}
		return 0
	}
	启动配置, 错误 := 配置模块包.加载参数(操作系统包.启动参数[1:])
	if 错误判断包.是否属于(错误, 命令行参数包.请求帮助错误) {
		配置模块包.输出使用说明(操作系统包.标准错误输出)
		return 0
	}
	if 错误 != nil {
		日志记录器.记录错误("读取启动配置失败", "module", "main", "error", 错误)
		return 1
	}
	执行上下文, 停止 := 系统信号包.创建信号上下文(上下文包.后台上下文(), 操作系统包.中断信号, 系统调用包.终止信号)
	defer 停止()
	// 标准输出 管道中断时通过 写入 错误进入资源清理，不由 管道断开信号 直接终止进程。
	系统信号包.忽略(系统调用包.管道断开信号)
	输入, 输出, 错误 := 传输模块包.打开标准输入输出()
	if 错误 != nil {
		日志记录器.记录错误("初始化标准输入输出失败", "module", "main", "error", 错误)
		return 1
	}
	if 错误 := 应用模块包.执行(执行上下文, 启动配置, 输入, 输出, 日志记录器); 错误 != nil {
		日志记录器.记录错误("服务退出失败", "module", "main", "error", 错误)
		return 1
	}
	return 0
}
```
