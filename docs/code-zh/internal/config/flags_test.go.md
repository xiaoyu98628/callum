```go
package 配置模块

import (
	字节包 "bytes"
	错误判断包 "errors"
	命令行参数包 "flag"
	字符串包 "strings"
	测试包 "testing"
	时间包 "time"
)

func Test运行时参数集(测试器 *测试包.测试器) {
	目录 := 测试器.创建临时目录()
	启动配置, 错误 := 加载参数([]string{"--data-dir", 目录, "--max-concurrent-runs", "2", "--call-timeout", "30s", "--tool-timeout", "10s"})
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 启动配置.数据目录 != 目录 || 启动配置.最大并发执行集 != 2 || 启动配置.调用超时 != 30*时间包.秒 || 启动配置.工具超时 != 10*时间包.秒 {
		测试器.格式化报告致命错误("参数未生效: %+v", 启动配置)
	}
	for _, 参数 := range [][]string{{"--max-concurrent-runs", "0"}, {"--max-concurrent-runs", "65"}, {"--call-timeout", "0s"}, {"--call-timeout", "bad"}, {"unknown"}, {"--unknown"}, {"--approval-mode", "always"}, {"--approval-policy", "auto-read"}, {"--approval-policy", "bad"}, {"--approval-policy", ""}, {"--approvals-reviewer", "auto_review"}, {"--approvals-reviewer", ""}, {"--approval-timeout", "1m"}, {"--tool-timeout", "-1s"}} {
		if _, 错误 := 加载参数(参数); 错误 == nil {
			测试器.格式化报告致命错误("错误参数被接受: %v", 参数)
		}
	}
	if _, 错误 := 加载参数([]string{"--help"}); !错误判断包.是否属于(错误, 命令行参数包.请求帮助错误) {
		测试器.格式化报告致命错误("帮助错误: %v", 错误)
	}
}

func Test审批设置不是启动参数集(测试器 *测试包.测试器) {
	for _, 参数 := range [][]string{{"--approval-policy", "on-request"}, {"--approvals-reviewer", "user"}, {"--approval-mode", "always"}} {
		if _, 错误 := 加载参数(append([]string{"--data-dir", 测试器.创建临时目录()}, 参数...)); 错误 == nil {
			测试器.格式化报告致命错误("接受已迁入数据库的启动参数: %v", 参数)
		}
	}
	var 用量 字节包.缓冲区
	输出使用说明(&用量)
	for _, 名称 := range []string{"-approval-policy", "-approvals-reviewer", "-approval-mode", "-sandbox"} {
		if 字符串包.包含(用量.字符串(), 名称) {
			测试器.格式化报告致命错误("帮助仍含无效参数 %s", 名称)
		}
	}
}

func Test随附运行时目录是显式并且绝对(测试器 *测试包.测试器) {
	目录 := 测试器.创建临时目录()
	启动配置, 错误 := 加载参数([]string{"--data-dir", 目录, "--runtime-dir", 目录})
	if 错误 != nil || 启动配置.运行时目录 != 目录 {
		测试器.格式化报告致命错误("运行时资源参数未传入装配: %+v %v", 启动配置, 错误)
	}
	if _, 错误 := 加载参数([]string{"--data-dir", 目录, "--runtime-dir", "resources/runtimes"}); 错误 == nil {
		测试器.报告致命错误("接受了依赖进程工作目录的运行时路径")
	}
}
```
