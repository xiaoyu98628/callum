```go
package 应用模块

import (
	上下文包 "context"
	错误判断包 "errors"
	格式化包 "fmt"
	结构化日志包 "log/slog"
	HTTP包 "net/http"
	操作系统包 "os"
	文件路径包 "path/filepath"
	同步包 "sync"
	时间包 "time"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	审批包 "github.com/xiaoyu98628/callum/internal/agent/approval"
	事件集包 "github.com/xiaoyu98628/callum/internal/agent/events"
	智能体执行器包 "github.com/xiaoyu98628/callum/internal/agent/executor"
	智能体服务包 "github.com/xiaoyu98628/callum/internal/agent/service"
	附件包 "github.com/xiaoyu98628/callum/internal/attachment"
	配置模块包 "github.com/xiaoyu98628/callum/internal/config"
	会话服务包 "github.com/xiaoyu98628/callum/internal/conversation/service"
	凭据包 "github.com/xiaoyu98628/callum/internal/credentials"
	系统包 "github.com/xiaoyu98628/callum/internal/credentials/system"
	推理调用包 "github.com/xiaoyu98628/callum/internal/inference"
	执行器包 "github.com/xiaoyu98628/callum/internal/inference/executor"
	MCP包 "github.com/xiaoyu98628/callum/internal/mcp"
	供应商服务包 "github.com/xiaoyu98628/callum/internal/model/provider/service"
	运行时资源包 "github.com/xiaoyu98628/callum/internal/runtimeassets"
	设置包 "github.com/xiaoyu98628/callum/internal/settings"
	设置服务包 "github.com/xiaoyu98628/callum/internal/settings/service"
	技能包 "github.com/xiaoyu98628/callum/internal/skills"
	SQLite包 "github.com/xiaoyu98628/callum/internal/storage/sqlite"
	智能体仓储包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/agent"
	目录管理包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/catalog"
	会话仓储包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/conversation"
	MCP仓储包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/mcp"
	设置仓储包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/settings"
	工作区仓储包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/workspace"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	打开附件包 "github.com/xiaoyu98628/callum/internal/tool/builtin/attachment/open"
	命令包 "github.com/xiaoyu98628/callum/internal/tool/builtin/command"
	文件系统包 "github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem"
	交付物包 "github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/artifact"
	运行时工具包 "github.com/xiaoyu98628/callum/internal/tool/builtin/runtime"
	工具执行器包 "github.com/xiaoyu98628/callum/internal/tool/executor"
	注册表包 "github.com/xiaoyu98628/callum/internal/tool/registry"
	工作区服务包 "github.com/xiaoyu98628/callum/internal/workspace/service"
)

// 运行时 持有业务服务、共享调度器、事件发布器和存储，不依赖通信连接。
// 创建者必须 关闭；借用的业务服务不得在 关闭 返回后继续使用。
type 运行时 struct {
	执行上下文               上下文包.上下文
	取消函数            上下文包.取消函数
	日志记录器            *结构化日志包.日志记录器
	数据库                *SQLite包.数据库
	所有权         *SQLite包.所有权
	HTTP传输     *HTTP包.传输
	MCP传输      *HTTP包.传输
	MCPOAuth传输 *HTTP包.传输
	MCP               *MCP包.服务
	管理器           *智能体执行器包.管理器
	事件集            *事件集包.事件中心
	供应商集         *供应商服务包.服务
	工作区集        *工作区服务包.服务
	会话集     *会话服务包.服务
	引擎            *智能体服务包.引擎
	审批集         *审批包.服务
	设置          *设置服务包.服务
	凭据       *凭据包.凭据保险库
	附件集       *附件包.存储
	附件访问  *附件包.附件访问器
	技能            *技能包.存储
	调用集             *执行器包.管理器
	互斥锁                同步包.互斥锁
	已关闭            bool
	会话连接集          同步包.等待组
	关闭一次         同步包.一次
	关闭错误          error
}

// 新建 装配与协议无关的运行时；执行上下文 是应用运行上下文，初始化使用独立短超时。
// 部分初始化失败时关闭此前取得的资源，且不覆盖原始错误。
func 新建(执行上下文 上下文包.上下文, 启动配置 配置模块包.配置, 日志记录器 *结构化日志包.日志记录器) (_ *运行时, 返回错误 error) {
	return 新建运行时(执行上下文, 启动配置, 日志记录器, 系统包.新建())
}

// 新建运行时 允许包内测试注入隔离凭据库，生产入口始终使用系统凭据库。
func 新建运行时(执行上下文 上下文包.上下文, 启动配置 配置模块包.配置, 日志记录器 *结构化日志包.日志记录器, 系统凭据库 凭据包.系统凭据库) (_ *运行时, 返回错误 error) {
	if 执行上下文 == nil || 日志记录器 == nil || !文件路径包.是否绝对路径(启动配置.数据库路径) || 启动配置.最大并发执行集 < 1 || 启动配置.最大并发执行集 > 64 || 启动配置.调用超时 <= 0 || 启动配置.工具超时 <= 0 {
		return nil, 错误判断包.新建("应用运行配置无效")
	}
	if 错误 := 执行上下文.获取错误(); 错误 != nil {
		return nil, 错误
	}
	运行时执行上下文, 取消函数 := 上下文包.创建可取消上下文(执行上下文)
	当前读取器 := &运行时{执行上下文: 运行时执行上下文, 取消函数: 取消函数, 日志记录器: 日志记录器}
	defer func() {
		if 返回错误 != nil {
			返回错误 = 错误判断包.合并(返回错误, 当前读取器.关闭())
		}
	}()
	var 错误 error
	var 资源包 *运行时资源包.资源包
	if 启动配置.运行时目录 != "" {
		// 完整资源树校验的文件 标识/打开标志 独立计时，不能消耗后续数据库初始化预算。
		资源执行上下文, 资源取消 := 上下文包.创建超时上下文(运行时执行上下文, 30*时间包.秒)
		资源包, 错误 = 运行时资源包.加载(资源执行上下文, 启动配置.运行时目录)
		资源取消()
		if 错误 != nil {
			return nil, 格式化包.格式化创建错误("加载内置运行时: %w", 错误)
		}
	}
	初始化执行上下文, 初始化取消 := 上下文包.创建超时上下文(运行时执行上下文, 10*时间包.秒)
	defer 初始化取消()
	// 实例锁覆盖迁移与恢复，另一个运行时不能在启动失败前改写活动实例的结构。
	当前读取器.所有权, 错误 = SQLite包.获取所有权(启动配置.数据库路径)
	if 错误 != nil {
		return nil, 错误
	}
	if 错误 := 命令包.恢复沙箱(初始化执行上下文, 文件路径包.目录(启动配置.数据库路径)); 错误 != nil {
		return nil, 错误
	}
	当前读取器.数据库, 错误 = SQLite包.打开(初始化执行上下文, 启动配置.数据库路径)
	if 错误 != nil {
		return nil, 错误
	}
	规范化目录, 错误 := 文件路径包.求值符号链接集(文件路径包.目录(启动配置.数据库路径))
	if 错误 != nil {
		return nil, 错误
	}
	当前读取器.凭据, 错误 = 凭据包.新建(系统凭据库, 文件路径包.合并(规范化目录, 文件路径包.基准(启动配置.数据库路径)))
	if 错误 != nil {
		return nil, 错误
	}
	if 错误 := 目录管理包.迁移凭据(初始化执行上下文, 当前读取器.数据库.数据库, 当前读取器.凭据); 错误 != nil {
		return nil, 错误
	}
	设置仓储, 错误 := 设置仓储包.新建仓储(当前读取器.数据库.数据库)
	if 错误 != nil {
		return nil, 错误
	}
	当前读取器.设置, 错误 = 设置服务包.新建(设置仓储)
	if 错误 != nil {
		return nil, 错误
	}
	for _, 键 := range []string{设置包.执行键, 设置包.压缩键, 设置包.常规键} {
		if _, 错误 := 当前读取器.设置.获取(初始化执行上下文, 键); 错误 != nil {
			return nil, 错误
		}
	}
	会话仓储, 错误 := 会话仓储包.新建仓储(当前读取器.数据库.数据库)
	if 错误 != nil {
		return nil, 错误
	}
	执行仓储, 错误 := 智能体仓储包.新建仓储(当前读取器.数据库.数据库)
	if 错误 != nil {
		return nil, 错误
	}
	if 错误 := 执行仓储.恢复(初始化执行上下文); 错误 != nil {
		return nil, 错误
	}
	当前读取器.附件集, 错误 = 附件包.打开(初始化执行上下文, 规范化目录, func(执行上下文 上下文包.上下文, 会话标识, 执行标识 string) (bool, error) {
		运行, 错误 := 执行仓储.获取(执行上下文, 执行标识)
		if 错误判断包.是否属于(错误, 智能体包.未找到错误) {
			return 会话服务包.是后续消息附件所属对象(会话仓储.后续消息(执行上下文, 会话标识, 执行标识))
		}
		if 错误 != nil {
			return false, 错误
		}
		if 运行.会话标识 != 会话标识 {
			return false, 错误判断包.新建("附件恢复记录的会话归属不匹配")
		}
		return true, nil
	}, 日志记录器)
	if 错误 != nil {
		return nil, 错误
	}
	工作区仓储, 错误 := 工作区仓储包.新建仓储(当前读取器.数据库.数据库)
	if 错误 != nil {
		return nil, 错误
	}
	当前读取器.工作区集, 错误 = 工作区服务包.新建(工作区仓储)
	if 错误 != nil {
		return nil, 错误
	}
	仓储, 错误 := 目录管理包.新建仓储(当前读取器.数据库.数据库, 当前读取器.凭据)
	if 错误 != nil {
		return nil, 错误
	}
	当前读取器.供应商集, 错误 = 供应商服务包.新建(仓储)
	if 错误 != nil {
		return nil, 错误
	}
	客户端, HTTP传输 := 新建HTTP客户端(启动配置.最大并发执行集)
	当前读取器.HTTP传输 = HTTP传输
	MCP客户端, MCP传输 := MCP包.HTTP客户端(false)
	当前读取器.MCP传输 = MCP传输
	MCPOAuth客户端, MCPOAuth传输 := MCP包.HTTP客户端(true)
	当前读取器.MCPOAuth传输 = MCPOAuth传输
	当前读取器.MCP, 错误 = MCP包.新建(初始化执行上下文, 运行时执行上下文, MCP仓储包.新建(当前读取器.数据库.数据库, 当前读取器.凭据), MCP客户端, MCPOAuth客户端, 日志记录器)
	if 错误 != nil {
		return nil, 错误
	}
	当前读取器.管理器, 错误 = 智能体执行器包.新建(运行时执行上下文, 启动配置.最大并发执行集, 启动配置.调用超时)
	if 错误 != nil {
		return nil, 错误
	}
	// 每个订阅最多排队 64 个事件，运行时最多持有 32 个订阅，无消费者时也可执行。
	当前读取器.事件集, 错误 = 事件集包.新建(64, 32)
	if 错误 != nil {
		return nil, 错误
	}
	当前读取器.调用集, 错误 = 执行器包.新建带有调度器(当前读取器.管理器, 新建模型解析器(当前读取器.供应商集, 客户端), func(事件 推理调用包.事件) error {
		return 当前读取器.事件集.发布(推理调用包.公开事件(事件))
	})
	if 错误 != nil {
		return nil, 错误
	}
	当前读取器.审批集, 错误 = 审批包.新建(执行仓储, 执行仓储.获取, 当前读取器.事件集.发布)
	if 错误 != nil {
		return nil, 错误
	}
	受保护目录, 错误 := 文件路径包.求值符号链接集(文件路径包.目录(启动配置.数据库路径))
	if 错误 != nil {
		return nil, 格式化包.格式化创建错误("解析应用数据目录: %w", 错误)
	}
	当前读取器.技能, 错误 = 技能包.打开(初始化执行上下文, 规范化目录, 日志记录器)
	if 错误 != nil {
		return nil, 错误
	}

	保护 := 工具包.受保护数据{目录: 受保护目录, 数据库: 文件路径包.合并(受保护目录, 文件路径包.基准(启动配置.数据库路径)), 只读目录集: []string{当前读取器.技能.获取目录()}}
	if 资源包 != nil {
		预期运行时 := 文件路径包.合并(受保护目录, "runtimes")
		if (保护.包含(资源包.获取目录()) && 资源包.获取目录() != 预期运行时) || (工具包.受保护数据{目录: 资源包.获取目录()}).包含(受保护目录) {
			return nil, 错误判断包.新建("运行时资源与私有数据目录不得重叠")
		}
		保护.只读目录集 = append(保护.只读目录集, 资源包.获取目录())
	}
	注册表 := 注册表包.新建()
	if 资源包 != nil && 资源包.存在工作区依赖集() {
		if 错误 := 注册表.注册(运行时工具包.新建(资源包)); 错误 != nil {
			return nil, 错误
		}
	}
	当前读取器.附件访问, 错误 = 附件包.打开访问(初始化执行上下文, 规范化目录, 当前读取器.附件集, 会话仓储.附件)
	if 错误 != nil {
		return nil, 错误
	}
	if 错误 := 注册表.注册(打开附件包.新建(当前读取器.附件访问)); 错误 != nil {
		return nil, 错误
	}
	for _, 实例 := range 交付物包.工具集(当前读取器.附件访问, 保护) {
		if 错误 := 注册表.注册(实例); 错误 != nil {
			return nil, 错误
		}
	}
	for _, 实例 := range append(文件系统包.工具集(保护), 命令包.工具集带有运行时(操作系统包.环境变量集(), 保护, 资源包)...) {
		if 错误 := 注册表.注册(实例); 错误 != nil {
			return nil, 错误
		}
	}
	工具集, 错误 := 工具执行器包.新建(注册表, 启动配置.工具超时)
	if 错误 != nil {
		return nil, 错误
	}
	当前读取器.引擎, 错误 = 智能体服务包.新建(执行仓储, 会话仓储, 当前读取器.管理器, 当前读取器.事件集.发布, 日志记录器, 智能体服务包.选项集{工具集: 工具集, 额外工具集: 当前读取器.MCP.工具集快照, 审批集: 当前读取器.审批集, 工作区: 当前读取器.工作区集.获取, 技能: 当前读取器.技能.上下文, 附件集: 当前读取器.附件访问, 调用超时: 启动配置.调用超时})
	if 错误 != nil {
		return nil, 错误
	}
	当前读取器.会话集, 错误 = 会话服务包.新建(会话仓储, 新建会话解析器(当前读取器.供应商集, 客户端, 当前读取器.附件集), 当前读取器.管理器, 当前读取器.引擎, 当前读取器.附件集, 当前读取器.技能, 当前读取器.事件集.发布, 当前读取器.日志记录器)
	if 错误 != nil {
		return nil, 错误
	}
	return 当前读取器, nil
}

// 供应商集 返回运行时持有的厂商与模型配置服务。
func (当前运行时 *运行时) 获取供应商集() *供应商服务包.服务 { return 当前运行时.供应商集 }

// 工作区集 返回运行时持有的工作区服务。
func (当前运行时 *运行时) 获取工作区集() *工作区服务包.服务 { return 当前运行时.工作区集 }

// 会话集 返回运行时持有的会话服务。
func (当前运行时 *运行时) 获取会话集() *会话服务包.服务 { return 当前运行时.会话集 }

// 审批集 返回与通信协议无关的审批服务。
func (当前运行时 *运行时) 获取审批集() *审批包.服务 { return 当前运行时.审批集 }

// 设置 返回数据库应用设置服务；更新成功后由后续受理执行读取新配置。
func (当前运行时 *运行时) 获取设置() *设置服务包.服务 { return 当前运行时.设置 }

// 执行集 返回运行时持有的持久化执行查询服务。
func (当前运行时 *运行时) 执行集() *智能体服务包.引擎 { return 当前运行时.引擎 }

// 调用 提供无历史模型调用，与普通对话共享并发和事件发布器。
func (当前运行时 *运行时) 调用(执行上下文 上下文包.上下文, 输入 推理调用包.请求, 已受理 func(string) error) error {
	return 当前运行时.调用集.启动(执行上下文, 输入, 已受理)
}

// 订阅 返回只读业务事件订阅，调用者必须关闭，断开订阅不会取消执行。
func (当前运行时 *运行时) 订阅(筛选 事件集包.筛选) (*事件集包.订阅, error) {
	当前运行时.互斥锁.加锁()
	defer 当前运行时.互斥锁.解锁()
	if 当前运行时.已关闭 || 当前运行时.执行上下文.获取错误() != nil {
		return nil, 事件集包.已关闭错误
	}
	return 当前运行时.事件集.订阅(筛选)
}

// 取消执行 请求取消指定执行，不等待结束；不存在或已确定终态时返回 false。
func (当前运行时 *运行时) 取消执行(标识 string) bool { return 当前运行时.管理器.取消执行(标识) }

// 关闭 停止入口、取消并等待任务及连接，之后释放网络、数据库和实例锁，可重复调用。
func (当前运行时 *运行时) 关闭() error {
	当前运行时.关闭一次.执行(func() {
		当前运行时.互斥锁.加锁()
		当前运行时.已关闭 = true
		当前运行时.取消函数()
		当前运行时.互斥锁.解锁()
		if 当前运行时.管理器 != nil {
			当前运行时.管理器.关闭()
		}
		当前运行时.会话连接集.等待()
		if 当前运行时.事件集 != nil {
			当前运行时.事件集.关闭()
		}
		if 当前运行时.MCP != nil {
			当前运行时.MCP.关闭()
		}
		if 当前运行时.MCPOAuth传输 != nil {
			当前运行时.MCPOAuth传输.关闭空闲连接()
		}
		if 当前运行时.MCP传输 != nil {
			当前运行时.MCP传输.关闭空闲连接()
		}
		if 当前运行时.HTTP传输 != nil {
			当前运行时.HTTP传输.关闭空闲连接()
		}
		if 当前运行时.凭据 != nil {
			当前运行时.关闭错误 = 错误判断包.合并(当前运行时.关闭错误, 当前运行时.凭据.关闭())
		}
		if 当前运行时.附件集 != nil {
			if 当前运行时.附件访问 != nil {
				当前运行时.关闭错误 = 错误判断包.合并(当前运行时.关闭错误, 当前运行时.附件访问.关闭())
			}
			当前运行时.关闭错误 = 错误判断包.合并(当前运行时.关闭错误, 当前运行时.附件集.关闭())
		}
		if 当前运行时.数据库 != nil {
			当前运行时.关闭错误 = 错误判断包.合并(当前运行时.关闭错误, 当前运行时.数据库.关闭())
		}
		if 当前运行时.所有权 != nil {
			当前运行时.关闭错误 = 错误判断包.合并(当前运行时.关闭错误, 当前运行时.所有权.关闭())
		}
	})
	return 当前运行时.关闭错误
}

func (当前运行时 *运行时) 开始会话连接() error {
	当前运行时.互斥锁.加锁()
	defer 当前运行时.互斥锁.解锁()
	if 当前运行时.已关闭 || 当前运行时.执行上下文.获取错误() != nil {
		return 智能体包.已关闭错误
	}
	当前运行时.会话连接集.添加(1)
	return nil
}
```
