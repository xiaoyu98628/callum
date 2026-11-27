```go
// 准备模块-运行时集 维护固定版本归档；默认离线，显式 --下载 才允许下载。
package main

import (
	上下文包 "context"
	错误判断包 "errors"
	命令行参数包 "flag"
	结构化日志包 "log/slog"
	操作系统包 "os"
	系统信号包 "os/signal"

	准备模块包 "github.com/xiaoyu98628/callum/internal/runtimeassets/prepare"
)

func main() {
	执行上下文, 取消函数 := 系统信号包.创建信号上下文(上下文包.后台上下文(), 操作系统包.中断信号)
	日志记录器 := 结构化日志包.新建(结构化日志包.新建JSON处理器(操作系统包.标准错误输出, nil)).附加字段("module", "runtimeassets.prepare")
	错误 := 准备模块包.运行命令(执行上下文, 操作系统包.启动参数[1:], 日志记录器, false)
	取消函数()
	if 错误判断包.是否属于(错误, 命令行参数包.请求帮助错误) {
		return
	}
	if 错误 != nil {
		日志记录器.记录错误("运行时资源准备失败", "error", 错误)
		操作系统包.退出进程(1)
	}
}
```
