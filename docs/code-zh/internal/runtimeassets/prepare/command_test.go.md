```go
package 准备模块

import (
	JSON包 "encoding/json"
	输入输出包 "io"
	结构化日志包 "log/slog"
	操作系统包 "os"
	文件路径包 "path/filepath"
	运行时包 "runtime"
	字符串包 "strings"
	测试包 "testing"

	运行时资源包 "github.com/xiaoyu98628/callum/internal/runtimeassets"
)

func Test离线命令无法启用下载集(测试器 *测试包.测试器) {
	目录 := 测试器.创建临时目录()
	来源 := 来源{版本: "fixed", URL: "https://example.invalid/archive", SHA256: 字符串包.重复("0", 64), 归档: "missing.tar.gz", 前缀: "root", 条目: "bin/interpreter"}
	数据, 错误 := JSON包.序列化(来源集{格式版本: 运行时资源包.格式版本, 目标集: []目标{{目录: "target", 平台: 运行时包.目标操作系统, 架构: 运行时包.目标架构, Python: 来源, 节点: 来源, Git: Git来源{来源: 来源, 进程执行路径: "libexec", 模板路径: "templates", 配置路径: "etc/gitconfig"}}}})
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	路径 := 文件路径包.合并(目录, "sources.json")
	if 错误 := 操作系统包.写入文件(路径, 数据, 0600); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	输出 := 文件路径包.合并(目录, "unpacked")
	参数 := []string{"--sources", 路径, "--output", 输出, "--cache", 目录, "--download"}
	错误 = 运行命令(测试器.上下文(), 参数, 结构化日志包.新建(结构化日志包.新建JSON处理器(输入输出包.丢弃, nil)), true)
	if 错误 == nil || !字符串包.包含(错误.错误文本(), "仅支持离线归档") {
		测试器.格式化报告致命错误("客户端准备入口接受了下载参数: %v", 错误)
	}
	if _, 错误 := 操作系统包.状态信息(输出); !操作系统包.是否不存在(错误) {
		测试器.格式化报告致命错误("拒绝参数前创建了资源目录: %v", 错误)
	}
}
```
