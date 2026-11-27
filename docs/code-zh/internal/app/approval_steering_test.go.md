```go
package 应用模块

import (
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"
	操作系统包 "os"
	文件路径包 "path/filepath"
	字符串包 "strings"
	原子操作包 "sync/atomic"
	测试包 "testing"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	事件集包 "github.com/xiaoyu98628/callum/internal/agent/events"
	会话包 "github.com/xiaoyu98628/callum/internal/conversation"
	会话服务包 "github.com/xiaoyu98628/callum/internal/conversation/service"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	设置包 "github.com/xiaoyu98628/callum/internal/settings"
	会话仓储包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/conversation"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	策略包 "github.com/xiaoyu98628/callum/internal/tool/policy"
)

func Test自动审核包含已生效引导(测试器 *测试包.测试器) {
	for _, 场景 := range []string{"normal", "resume", "during_review"} {
		测试器.执行(场景, func(测试器 *测试包.测试器) {
			继续 := 场景 == "resume"
			运行时, _ := 运行时测试夹具(测试器, 1)
			记录, 错误 := 运行时.获取设置().获取(测试器.上下文(), 设置包.执行键)
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			原始, 错误 := JSON包.序列化(设置包.执行{审批策略: 策略包.开启请求, 审批审核方: 策略包.自动审核, 沙箱模式: 工具包.沙箱只读})
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			if _, 错误 := 运行时.获取设置().更新(测试器.上下文(), 设置包.执行键, 记录.修订号, 原始); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			项目, 错误 := 运行时.获取工作区集().创建(测试器.上下文(), "审批引导", 测试器.创建临时目录())
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			const 引导 = "现在仅允许创建 new.txt，不再创建 old.txt"
			var 阶段 原子操作包.整数32
			var main已引导, 审核已引导, 审核集 原子操作包.整数32
			var 已跳过 原子操作包.布尔
			var 服务 *会话服务包.服务
			var 执行 会话包.执行
			客户端 := 操作结果测试模型(func(_ 上下文包.上下文, 请求 模型包.请求) (模型包.响应, error) {
				if 字符串包.存在前缀(请求.消息集[0].内容, "你独立审核") {
					审核集.添加(1)
					if 场景 == "during_review" {
						_, 错误 := 服务.添加后续消息(测试器.上下文(), 会话包.后续消息输入{会话标识: 执行.会话.标识, 预期执行标识: 执行.执行.标识, 模式: "steer", 内容: []模型包.内容块{{类型: "text", 文本: 引导}}})
						return 模型包.响应{文本: `{"decision":"approved","reason":"受控测试批准旧操作"}`}, 错误
					}
					if 字符串包.包含(请求.消息集[1].内容, 引导) {
						审核已引导.添加(1)
					}
					return 模型包.响应{文本: `{"decision":"rejected","reason":"受控测试拒绝执行"}`}, nil
				}
				if len(请求.工具集) == 0 {
					return 模型包.响应{文本: "审批引导"}, nil
				}
				调用标识, 名称 := "initial-patch", "new.txt"
				if 阶段.加载() == 1 {
					调用标识, 名称 = "resumed-patch", "new-resumed.txt"
				}
				for _, 映射 := range 请求.消息集 {
					if 字符串包.包含(映射.内容, 引导) {
						main已引导.添加(1)
					}
					for _, 乙 := range 映射.内容块集 {
						if 字符串包.包含(乙.文本, 引导) {
							main已引导.添加(1)
						}
					}
				}
				for _, 映射 := range 请求.消息集 {
					if 映射.角色 == 模型包.角色工具 && 映射.工具调用标识 == 调用标识 {
						if 字符串包.包含(映射.内容, `"kind":"steered"`) {
							已跳过.存储(true)
						}
						if 继续 && 阶段.加载() == 0 {
							return 模型包.响应{}, 上下文包.已取消错误
						}
						return 模型包.响应{文本: "已结束"}, nil
					}
				}
				参数, 错误 := JSON包.序列化(map[string]any{"patch": "*** Begin Patch\n*** Add File: " + 名称 + "\n+受控内容\n*** End Patch\n", "base_versions": []any{}})
				return 模型包.响应{工具调用集: []模型包.工具调用{{标识: 调用标识, 名称: "apply_patch", 参数: 参数}}}, 错误
			})
			仓储, 错误 := 会话仓储包.新建仓储(运行时.数据库.数据库)
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			标识 := 策略测试标识(测试器)
			服务, 错误 = 会话服务包.新建(仓储, func(上下文包.上下文, string) (智能体包.已解析, error) {
				return 智能体包.已解析{客户端: 客户端, 快照: 模型包.快照{标识: 标识, 远程模型标识: "controlled"}, 协议: "chatcompletions", 上下文窗口: 1000000}, nil
			}, 运行时.管理器, 运行时.引擎, 运行时.附件集, 运行时.技能, 运行时.事件集.发布, 运行时.日志记录器)
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			订阅, 错误 := 运行时.订阅(事件集包.筛选{})
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			defer 订阅.关闭()
			错误 = 服务.发送(测试器.上下文(), 会话包.发送{模型标识: 标识, 工作区标识: &项目.标识, 流: new(false), 内容: []模型包.内容块{{类型: "text", 文本: "请创建 old.txt"}}}, func(本次执行 会话包.执行) error {
				执行 = 本次执行
				if 场景 == "during_review" {
					return nil
				}
				_, 错误 := 服务.添加后续消息(测试器.上下文(), 会话包.后续消息输入{会话标识: 本次执行.会话.标识, 预期执行标识: 本次执行.执行.标识, 模式: "steer", 内容: []模型包.内容块{{类型: "text", 文本: 引导}}})
				return 错误
			})
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			终态 := 等待操作结果(测试器, 运行时, 订阅, 执行.执行.标识, nil)
			预期审核集 := int32(1)
			if 继续 {
				if 终态.类型 != "run.cancelled" {
					测试器.格式化报告致命错误("执行没有停止: %+v", 终态)
				}
				阶段.存储(1)
				if 错误 := 服务.继续(测试器.上下文(), 会话包.继续{会话标识: 执行.会话.标识, 执行标识: 执行.执行.标识, 模型标识: 标识}, func(会话包.执行) error { return nil }); 错误 != nil {
					测试器.报告致命错误(错误)
				}
				终态 = 等待操作结果(测试器, 运行时, 订阅, 执行.执行.标识, nil)
				预期审核集 = 2
			}
			预期已引导 := 预期审核集
			if 场景 == "during_review" {
				预期已引导 = 0
				if !已跳过.加载() {
					测试器.报告致命错误("审核期间出现新引导后，旧操作没有被跳过")
				}
			}
			if 终态.类型 != "run.completed" || main已引导.加载() == 0 || 审核集.加载() != 预期审核集 || 审核已引导.加载() != 预期已引导 {
				测试器.格式化报告致命错误("审批未使用生效引导: terminal=%s main=%d reviews=%d", 终态.类型, main已引导.加载(), 审核已引导.加载())
			}
			for _, 名称 := range []string{"old.txt", "new.txt", "new-resumed.txt"} {
				if _, 错误 := 操作系统包.状态信息(文件路径包.合并(项目.根路径, 名称)); !错误判断包.是否属于(错误, 操作系统包.不存在错误) {
					测试器.格式化报告致命错误("被拒绝操作产生了文件: %s %v", 名称, 错误)
				}
			}
		})
	}
}
```
