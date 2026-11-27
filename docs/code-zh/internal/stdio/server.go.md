```go
package 标准输入输出

import (
	缓冲输入输出包 "bufio"
	字节包 "bytes"
	上下文包 "context"
	错误判断包 "errors"
	格式化包 "fmt"
	结构化日志包 "log/slog"
	同步包 "sync"

	审批包 "github.com/xiaoyu98628/callum/internal/agent/approval"
	智能体服务包 "github.com/xiaoyu98628/callum/internal/agent/service"
	会话服务包 "github.com/xiaoyu98628/callum/internal/conversation/service"
	推理调用包 "github.com/xiaoyu98628/callum/internal/inference"
	执行器包 "github.com/xiaoyu98628/callum/internal/inference/executor"
	供应商服务包 "github.com/xiaoyu98628/callum/internal/model/provider/service"
	设置服务包 "github.com/xiaoyu98628/callum/internal/settings/service"
	技能包 "github.com/xiaoyu98628/callum/internal/skills"
	审批处理器包 "github.com/xiaoyu98628/callum/internal/stdio/approval"
	目录管理包 "github.com/xiaoyu98628/callum/internal/stdio/catalog"
	会话处理器包 "github.com/xiaoyu98628/callum/internal/stdio/conversation"
	执行包 "github.com/xiaoyu98628/callum/internal/stdio/execution"
	MCP处理器包 "github.com/xiaoyu98628/callum/internal/stdio/mcp"
	设置处理器包 "github.com/xiaoyu98628/callum/internal/stdio/settings"
	技能处理器包 "github.com/xiaoyu98628/callum/internal/stdio/skills"
	传输模块包 "github.com/xiaoyu98628/callum/internal/stdio/transport"
	用户输入处理器包 "github.com/xiaoyu98628/callum/internal/stdio/userinput"
	工作区处理器包 "github.com/xiaoyu98628/callum/internal/stdio/workspace"
	工作区服务包 "github.com/xiaoyu98628/callum/internal/workspace/service"
)

// 服务端 只负责协议处理，配置业务服务与执行管理器均由装配层注入。
// 连接、任务、数据库及 HTTP 资源的创建与关闭顺序由调用方管理。
type 服务端 struct {
	目录管理  *目录管理包.处理器
	执行    *执行包.处理器
	工作区   *工作区处理器包.处理器
	会话    *会话处理器包.处理器
	日志记录器 *结构化日志包.日志记录器
	审批集   *审批处理器包.处理器
	设置    *设置处理器包.处理器
	技能    *技能处理器包.处理器
	输入集   *用户输入处理器包.处理器
	MCP   *MCP处理器包.处理器
}

// 新建带有会话集 装配普通对话接口；所有服务和资源仍由 应用模块 持有。
func 新建带有会话集(供应商集 *供应商服务包.服务, 执行集 *执行器包.管理器, 工作区集 *工作区服务包.服务, 会话集 *会话服务包.服务, 持久化 *智能体服务包.引擎, 审批集 *审批包.服务, 设置 *设置服务包.服务, 技能存储 *技能包.存储, 已受理 func(string) error, 日志记录器 *结构化日志包.日志记录器, MCP处理器 *MCP处理器包.处理器) (*服务端, error) {
	if 工作区集 == nil || 会话集 == nil || 持久化 == nil || 审批集 == nil || 设置 == nil || 技能存储 == nil || 已受理 == nil {
		return nil, 错误判断包.新建("对话服务不能为空")
	}
	当前状态, 错误 := 新建(供应商集, 执行集, 日志记录器)
	if 错误 != nil {
		return nil, 错误
	}
	当前状态.MCP = MCP处理器
	当前状态.审批集 = 审批处理器包.新建(审批集)
	当前状态.设置 = 设置处理器包.新建(设置)
	当前状态.技能 = 技能处理器包.新建(技能存储)
	当前状态.输入集 = 用户输入处理器包.新建(持久化)
	当前状态.工作区 = 工作区处理器包.新建(工作区集)
	当前状态.会话 = 会话处理器包.新建(会话集, 已受理)
	当前状态.执行 = 执行包.新建持久化(执行集, 持久化, 已受理)
	return 当前状态, nil
}

// 新建 创建协议处理对象，不创建或关闭外部资源。
func 新建(供应商集 *供应商服务包.服务, 执行集 *执行器包.管理器, 日志记录器 *结构化日志包.日志记录器) (*服务端, error) {
	if 供应商集 == nil || 执行集 == nil || 日志记录器 == nil {
		return nil, 错误判断包.新建("通信服务初始化参数无效")
	}
	return &服务端{目录管理包: 目录管理包.新建(供应商集), 执行包: 执行包.新建(执行集), 日志记录器: 日志记录器}, nil
}

func (当前服务端 *服务端) 信息() map[string]any {
	上限, 超时 := 当前服务端.执行.上限集()
	return map[string]any{"service": "callum", "protocol_version": 1, "max_concurrent_runs": 上限,
		"call_timeout_ms": 超时.毫秒(), "max_request_bytes": 传输模块包.最大请求字节,
		"max_response_bytes": 传输模块包.最大响应字节, "max_text_bytes": 推理调用包.最大文本字节}
}

// 就绪 将就绪通知排入连接，必须在该连接的事件转发启动前调用。
func (当前服务端 *服务端) 就绪(当前连接 *传输模块包.连接) error {
	if 当前连接 == nil {
		return 错误判断包.新建("协议连接不能为空")
	}
	return 当前连接.规范化错误(当前连接.通知("system.ready", 当前服务端.信息()))
}

// 提供服务 发送就绪通知并处理请求，资源仍由创建者关闭。
func (当前服务端 *服务端) 提供服务(当前连接 *传输模块包.连接) error {
	if 错误 := 当前服务端.就绪(当前连接); 错误 != nil {
		return 错误
	}
	return 当前服务端.提供服务请求集(当前连接)
}

// 提供服务请求集 持续处理请求直到 文件结束、关闭请求、取消或通信失败。
// 返回后创建者应停止本连接的任务和事件转发，再排空并关闭连接。
func (当前服务端 *服务端) 提供服务请求集(当前连接 *传输模块包.连接) error {
	if 当前连接 == nil {
		return 错误判断包.新建("协议连接不能为空")
	}
	所属对象, 取消函数 := 上下文包.创建可取消上下文(当前连接.上下文())
	待处理 := &网络请求集{执行上下文: 所属对象, 槽位集: make(chan struct{}, 4)}
	defer func() {
		取消函数()
		待处理.等待组.等待()
	}()
	扫描器 := 缓冲输入输出包.新建扫描器(当前连接.获取输入())
	扫描器.缓冲区(make([]byte, 4096), 传输模块包.最大请求字节+2)
	for 扫描器.扫描() {
		if 当前连接.上下文().获取错误() != nil {
			return nil
		}
		行 := 扫描器.字节()
		if len(行) > 传输模块包.最大请求字节 {
			return 错误判断包.新建("协议请求超过 1 MiB 限制")
		}
		if len(字节包.裁剪空间(行)) == 0 {
			continue
		}
		停止, 错误 := 当前服务端.句柄(当前连接, 待处理, 行)
		if 错误 != nil {
			return 当前连接.规范化错误(错误)
		}
		if 停止 {
			return nil
		}
	}
	if 错误 := 扫描器.获取错误(); 错误 != nil {
		return 当前连接.规范化错误(格式化包.格式化创建错误("读取协议输入失败: %w", 错误))
	}
	return nil
}

// 网络请求集 在同一协议连接内有界运行 MCP 网络操作，退出时取消并等待响应生产者。
type 网络请求集 struct {
	执行上下文 上下文包.上下文
	槽位集   chan struct{}
	等待组   同步包.等待组
}
```
