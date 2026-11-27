```go
package 服务

import (
	上下文包 "context"
	结构化日志包 "log/slog"
	同步包 "sync"
	原子操作包 "sync/atomic"
	时间包 "time"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	审批包 "github.com/xiaoyu98628/callum/internal/agent/approval"
	执行器包 "github.com/xiaoyu98628/callum/internal/agent/executor"
	附件包 "github.com/xiaoyu98628/callum/internal/attachment"
	会话包 "github.com/xiaoyu98628/callum/internal/conversation"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	工具执行器包 "github.com/xiaoyu98628/callum/internal/tool/executor"
	工作区包 "github.com/xiaoyu98628/callum/internal/workspace"
)

// 引擎 编排已提交的普通对话执行，不持有数据库事务或 HTTP 资源所有权。
type 引擎 struct {
	仓储    智能体包.仓储
	会话集   会话包.仓储
	管理器   *执行器包.管理器
	发出事件  func(智能体包.事件) error
	日志记录器 *结构化日志包.日志记录器
	选项集   选项集
	输入互斥锁 同步包.互斥锁
	输入信号集 map[string]chan struct{}
	引导互斥锁 同步包.互斥锁 // 只保护租约与状态的映射。
	引导    map[*执行器包.执行租约]*执行引导
	用量互斥锁 同步包.读写互斥锁
	用量    map[string]实时上下文用量
	用量修订号 原子操作包.整数64
}

// 选项集 保存运行时依赖；审批设置来自每次执行受理时持久化的快照。
type 选项集 struct {
	额外工具集 func(上下文包.上下文, bool, int64) []工具包.工具
	工具集   *工具执行器包.执行器
	审批集   *审批包.服务
	工作区   func(上下文包.上下文, string) (工作区包.工作区, error)
	技能    func(上下文包.上下文) (string, error)
	附件集   *附件包.附件访问器
	调用超时  时间包.时长
}

func 新建(仓储 智能体包.仓储, 会话集 会话包.仓储, 管理器 *执行器包.管理器, 发出事件 func(智能体包.事件) error, 日志记录器 *结构化日志包.日志记录器, 选项集 选项集) (*引擎, error) {
	if 仓储 == nil || 会话集 == nil || 管理器 == nil || 发出事件 == nil || 日志记录器 == nil || 选项集.调用超时 <= 0 || 选项集.工具集 == nil || 选项集.审批集 == nil || 选项集.工作区 == nil {
		return nil, 智能体包.无效错误
	}
	for _, 定义 := range 选项集.工具集.定义集() {
		if 定义.名称 == 智能体包.用户输入工具 {
			return nil, 智能体包.无效错误
		}
	}
	return &引擎{仓储: 仓储, 会话集: 会话集, 管理器: 管理器, 发出事件: 发出事件, 日志记录器: 日志记录器, 选项集: 选项集, 输入信号集: make(map[string]chan struct{}), 引导: make(map[*执行器包.执行租约]*执行引导), 用量: make(map[string]实时上下文用量)}, nil
}

func (当前引擎 *引擎) 事件(本次执行 会话包.执行, 名称, 步骤标识, 消息标识 string, 数据 any) error {
	事件 := 智能体包.事件{类型: 名称, 会话标识: 本次执行.会话.标识, 执行标识: 本次执行.执行.标识, 步骤标识: 步骤标识, 消息标识: 消息标识, 数据: 数据}
	if 名称 == "run.started" {
		事件.已移除消息标识集 = 本次执行.已移除消息标识集
	}
	return 当前引擎.发出事件(事件)
}

// 清理上下文 仅用于保存已发生操作的终态，避免取消导致清理写入被立即拒绝。
// 保留调用上下文的值，并以短超时限制关闭等待。
func 清理上下文(执行上下文 上下文包.上下文) (上下文包.上下文, 上下文包.取消函数) {
	return 上下文包.创建超时上下文(上下文包.无取消(执行上下文), 5*时间包.秒)
}

func (当前引擎 *引擎) 放弃(执行租约 *执行器包.执行租约, 本次执行 会话包.执行, 原因 error) {
	清理, 取消函数 := 清理上下文(执行租约.上下文())
	defer 取消函数()
	var 待处理 []会话包.后续消息
	if 状态 := 当前引擎.引导状态(执行租约); 状态 != nil {
		defer 当前引擎.移除引导(执行租约)
		待处理 = 当前引擎.加密封装引导(清理, 状态)
	}
	状态, 类别, 消息 := 失败(原因)
	当前时间 := 时间包.当前时间().Unix毫秒()
	本次执行.执行.状态, 本次执行.执行.错误类别, 本次执行.执行.错误消息, 本次执行.执行.更新时间, 本次执行.执行.结束时间 = 状态, &类别, &消息, 当前时间, &当前时间
	if 错误 := 当前引擎.仓储.结束(清理, 本次执行.执行); 错误 != nil {
		当前引擎.日志记录器.记录错误("保存未启动执行的终态失败", "module", "run", "run_id", 本次执行.执行.标识)
		return
	}
	当前引擎.引导已重新排队(本次执行, 待处理)
}
```
