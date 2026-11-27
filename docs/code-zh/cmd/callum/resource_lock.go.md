```go
package main

import (
	缓冲输入输出包 "bufio"
	字节包 "bytes"
	上下文包 "context"
	SHA256包 "crypto/sha256"
	十六进制包 "encoding/hex"
	JSON包 "encoding/json"
	错误判断包 "errors"
	命令行参数包 "flag"
	输入输出包 "io"
	结构化日志包 "log/slog"
	操作系统包 "os"
	系统信号包 "os/signal"
	文件路径包 "path/filepath"
	正则表达式包 "regexp"
	系统调用包 "syscall"
	时间包 "time"

	配置模块包 "github.com/xiaoyu98628/callum/internal/config"
	运行时资源包 "github.com/xiaoyu98628/callum/internal/runtimeassets"
	协议包 "github.com/xiaoyu98628/callum/internal/stdio/protocol"
	传输模块包 "github.com/xiaoyu98628/callum/internal/stdio/transport"
	SQLite包 "github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

var 运行时阶段名称 = 正则表达式包.必须编译(`^\.runtimes-install-[A-Za-z0-9]+$`)
var 运行时清单SHA256 = 正则表达式包.必须编译(`^[0-9a-f]{64}$`)

// 资源安装由客户端执行；此模式持有与服务相同的实例锁并校验安装目录，不打开数据库。
// 标准流关闭或进程终止即释放锁，避免客户端异常退出留下过期锁状态。
func 运行资源锁(参数 []string, 日志记录器 *结构化日志包.日志记录器) int {
	参数集 := 命令行参数包.新建命令行参数设置("resource-lock", 命令行参数包.继续开启错误)
	参数集.设置输出(输入输出包.丢弃)
	数据 := 参数集.字符串("data-dir", "", "待安装资源的应用数据目录")
	if 错误 := 参数集.解析(参数); 错误 != nil {
		if 错误判断包.是否属于(错误, 命令行参数包.请求帮助错误) {
			参数集.设置输出(操作系统包.标准错误输出)
			参数集.打印默认值()
			return 0
		}
		日志记录器.记录错误("资源锁参数无效", "module", "main")
		return 1
	}
	if *数据 == "" || 参数集.剩余参数数量() != 0 {
		日志记录器.记录错误("资源锁必须指定数据目录", "module", "main")
		return 1
	}
	启动配置, 错误 := 配置模块包.加载(*数据)
	if 错误 != nil {
		日志记录器.记录错误("读取资源锁配置失败", "module", "main")
		return 1
	}
	执行上下文, 停止 := 系统信号包.创建信号上下文(上下文包.后台上下文(), 操作系统包.中断信号, 系统调用包.终止信号)
	defer 停止()
	系统信号包.忽略(系统调用包.管道断开信号)
	输入, 输出, 错误 := 传输模块包.打开标准输入输出()
	if 错误 == nil {
		错误 = 持有资源加锁(执行上下文, 启动配置.数据库路径, 输入, 输出)
	}
	if 错误 != nil {
		日志记录器.记录错误("资源安装锁失败", "module", "main", "error", 错误)
		return 1
	}
	return 0
}

func 持有资源加锁(执行上下文 上下文包.上下文, 数据库 string, 输入 输入输出包.读取关闭器, 输出 输入输出包.写入关闭器) (返回错误 error) {
	// 输入输出在进入函数时转移所有权，即使实例锁获取失败也必须关闭。
	丙, 错误 := 传输模块包.新建(执行上下文, 输入, 输出)
	if 错误 != nil {
		return 错误判断包.合并(错误, 输入.关闭(), 输出.关闭())
	}
	defer func() { 返回错误 = 错误判断包.合并(返回错误, 丙.关闭()) }()
	所属对象, 错误 := SQLite包.获取所有权(数据库)
	if 错误 != nil {
		return 错误
	}
	defer func() { 返回错误 = 错误判断包.合并(返回错误, 所属对象.关闭()) }()
	数据目录, 错误 := 文件路径包.求值符号链接集(文件路径包.目录(数据库))
	if 错误 != nil {
		return 错误
	}
	if 错误 := 丙.通知("system.ready", map[string]any{"service": "callum", "protocol_version": 1, "max_request_bytes": 传输模块包.最大请求字节, "max_response_bytes": 传输模块包.最大响应字节, "resource_lock": true}); 错误 != nil {
		return 错误
	}
	扫描器 := 缓冲输入输出包.新建扫描器(丙.获取输入())
	扫描器.缓冲区(make([]byte, 4096), 传输模块包.最大请求字节+2)
	for 扫描器.扫描() {
		行 := 扫描器.字节()
		if len(行) > 传输模块包.最大请求字节 {
			return 错误判断包.新建("资源锁请求超过边界")
		}
		if len(字节包.裁剪空间(行)) == 0 {
			continue
		}
		请求, 无效 := 协议包.解析请求(行)
		if 无效 != nil {
			return 错误判断包.新建("资源锁请求无效")
		}
		var 参数 struct{}
		if 请求.方法 == "system.shutdown" && 协议包.解码参数(请求.参数, &参数) == nil {
			return 丙.回复(请求, map[string]bool{"shutdown": true}, nil)
		}
		if 请求.方法 == "runtime.validate" {
			if 错误 := 校验运行时目录(丙, 请求, 数据目录); 错误 != nil {
				return 错误
			}
			continue
		}
		if 错误 := 丙.回复(请求, nil, 协议包.方法未找到()); 错误 != nil {
			return 错误
		}
	}
	return 丙.规范化错误(扫描器.获取错误())
}

func 校验运行时目录(当前连接 *传输模块包.连接, 请求 协议包.请求, 数据目录 string) error {
	var 参数 struct {
		名称       string         `json:"name"`
		清单SHA256 JSON包.原始JSON消息 `json:"manifest_sha256"`
	}
	if 错误 := 协议包.解码参数(请求.参数, &参数); 错误 != nil {
		return 当前连接.回复(请求, nil, 错误)
	}
	if 参数.名称 != "runtimes" && !运行时阶段名称.匹配字符串(参数.名称) {
		return 当前连接.回复(请求, nil, 协议包.无效参数())
	}
	var 摘要 [SHA256包.大小]byte
	if 参数.清单SHA256 != nil {
		var 预期 string
		if 错误 := JSON包.反序列化(参数.清单SHA256, &预期); 错误 != nil || !运行时清单SHA256.匹配字符串(预期) {
			return 当前连接.回复(请求, nil, 协议包.无效参数())
		}
		if _, 错误 := 十六进制包.解码(摘要[:], []byte(预期)); 错误 != nil {
			return 当前连接.回复(请求, nil, 协议包.无效参数())
		}
	}
	路径 := 文件路径包.合并(数据目录, 参数.名称)
	信息, 错误 := 操作系统包.读取链接自身状态(路径)
	有效 := false
	if 错误 == nil && 信息.是否目录() {
		执行上下文, 取消函数 := 上下文包.创建超时上下文(当前连接.上下文(), 30*时间包.秒)
		if 参数.清单SHA256 == nil {
			_, 错误 = 运行时资源包.加载(执行上下文, 路径)
		} else {
			_, 错误 = 运行时资源包.加载带有清单SHA256(执行上下文, 路径, 摘要)
		}
		取消函数()
		if 错误判断包.是否属于(错误, 上下文包.已取消错误) || 错误判断包.是否属于(错误, 上下文包.超出截止时间错误) {
			return 当前连接.回复(请求, nil, &协议包.错误{代码: -32603, 消息: "运行时目录校验未完成"})
		}
		有效 = 错误 == nil
	}
	return 当前连接.回复(请求, map[string]bool{"valid": 有效}, nil)
}
```
