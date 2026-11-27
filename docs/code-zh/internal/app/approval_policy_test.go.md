```go
package 应用模块

import (
	字节包 "bytes"
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"
	原子操作包 "sync/atomic"
	测试包 "testing"
	时间包 "time"

	UUID包 "github.com/google/uuid"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	事件集包 "github.com/xiaoyu98628/callum/internal/agent/events"
	智能体服务包 "github.com/xiaoyu98628/callum/internal/agent/service"
	会话包 "github.com/xiaoyu98628/callum/internal/conversation"
	会话服务包 "github.com/xiaoyu98628/callum/internal/conversation/service"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	设置包 "github.com/xiaoyu98628/callum/internal/settings"
	智能体仓储包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/agent"
	会话仓储包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/conversation"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	工具执行器包 "github.com/xiaoyu98628/callum/internal/tool/executor"
	策略包 "github.com/xiaoyu98628/callum/internal/tool/policy"
	注册表包 "github.com/xiaoyu98628/callum/internal/tool/registry"
)

// 策略测试工具 只记录是否真正执行，用可信类别模拟尚未接入的操作，不执行外部操作。
type 策略测试工具 struct {
	影响集 []工具包.影响
	调用集 原子操作包.整数32
}

func (当前策略测试工具 *策略测试工具) 获取定义() 工具包.定义 {
	return 工具包.定义{名称: "test_operation", 说明: "审批判定测试", 参数: JSON包.原始JSON消息(`{"type":"object","additionalProperties":false}`), 来源: "test", 版本: "1", 影响集: 当前策略测试工具.影响集}
}
func (当前策略测试工具 *策略测试工具) 校验(参数 JSON包.原始JSON消息) (JSON包.原始JSON消息, error) {
	if string(参数) != "{}" {
		return nil, 工具包.无效错误
	}
	return JSON包.原始JSON消息(`{}`), nil
}
func (当前策略测试工具 *策略测试工具) 准备(上下文包.上下文, 工具包.范围, JSON包.原始JSON消息) (工具包.工具调用, error) {
	return 当前策略测试工具, nil
}
func (当前策略测试工具 *策略测试工具) 获取目标() *工具包.目标 { return nil }
func (当前策略测试工具 *策略测试工具) 关闭() error     { return nil }
func (当前策略测试工具 *策略测试工具) 执行(执行上下文 上下文包.上下文) (工具包.结果, error) {
	if 错误 := 执行上下文.获取错误(); 错误 != nil {
		return 工具包.结果{操作结果: 工具包.操作结果不已生效}, 错误
	}
	当前策略测试工具.调用集.添加(1)
	操作结果 := 工具包.操作结果已生效
	if 工具包.未确认操作结果(当前策略测试工具.影响集) == 工具包.操作结果无 {
		操作结果 = 工具包.操作结果无
	}
	return 工具包.结果{输出: JSON包.原始JSON消息(`{"executed":true}`), 操作结果: 操作结果}, nil
}

type 策略测试模型 struct {
	参数  JSON包.原始JSON消息
	已拒绝 bool
}

func (当前策略测试模型 策略测试模型) 生成(执行上下文 上下文包.上下文, 请求 模型包.请求) (模型包.响应, error) {
	if 错误 := 执行上下文.获取错误(); 错误 != nil {
		return 模型包.响应{}, 错误
	}
	最后 := 请求.消息集[len(请求.消息集)-1]
	if 最后.角色 == 模型包.角色工具 {
		if 字节包.包含([]byte(最后.内容), []byte(`"error"`)) != 当前策略测试模型.已拒绝 {
			return 模型包.响应{}, 错误判断包.新建("工具权限结果与预期不符")
		}
		return 模型包.响应{文本: "完成"}, nil
	}
	return 模型包.响应{工具调用集: []模型包.工具调用{{标识: "call-operation", 名称: "test_operation", 参数: 当前策略测试模型.参数}}}, nil
}
func (当前策略测试模型 策略测试模型) 流式生成(上下文包.上下文, 模型包.请求, func(模型包.事件) error) error {
	return 错误判断包.新建("本测试仅调用 Generate")
}

func Test工具审批策略集执行前(测试器 *测试包.测试器) {
	for _, 测试 := range []struct {
		名称   string
		策略   策略包.审批策略
		影响集  []工具包.影响
		参数   string
		动作   策略包.动作
		预期原因 string
	}{
		{"on_request_read", 策略包.开启请求, []工具包.影响{工具包.影响读取}, "{}", 策略包.允许, "read_only_operation"},
		{"on_request_write", 策略包.开启请求, []工具包.影响{工具包.影响写入}, "{}", 策略包.允许, "workspace_read_write_policy"},
		{"on_request_delete", 策略包.开启请求, []工具包.影响{工具包.影响破坏性}, "{}", 策略包.询问, "destructive_operation"},
		{"on_request_external", 策略包.开启请求, []工具包.影响{工具包.影响外部}, "{}", 策略包.询问, "external_operation"},
		{"on_request_unclassified", 策略包.开启请求, nil, "{}", 策略包.询问, "unclassified_operation"},
		{"always_read", 策略包.总是, []工具包.影响{工具包.影响读取}, "{}", 策略包.询问, "policy_always"},
		{"never_read", 策略包.从不, []工具包.影响{工具包.影响读取}, "{}", 策略包.允许, "read_only_operation"},
		{"never_write", 策略包.从不, []工具包.影响{工具包.影响写入}, "{}", 策略包.允许, "workspace_read_write_policy"},
		{"never_read_delete", 策略包.从不, []工具包.影响{工具包.影响读取, 工具包.影响破坏性}, "{}", 策略包.拒绝, "approval_required_but_disabled"},
		{"never_external", 策略包.从不, []工具包.影响{工具包.影响外部}, "{}", 策略包.拒绝, "approval_required_but_disabled"},
		{"never_unclassified", 策略包.从不, nil, "{}", 策略包.拒绝, "approval_required_but_disabled"},
		{"always_invalid_args", 策略包.总是, []工具包.影响{工具包.影响读取}, `{"effects":["read"]}`, 策略包.拒绝, "operation_validation_failed"},
	} {
		测试器.执行(测试.名称, func(测试器 *测试包.测试器) {
			运行时, 启动配置 := 运行时测试夹具(测试器, 1)
			设置运行时策略(测试器, 运行时, 测试.策略)
			实例 := &策略测试工具{影响集: 测试.影响集}
			工具集 := 注册表包.新建()
			if 错误 := 工具集.注册(实例); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			执行器, 错误 := 工具执行器包.新建(工具集, 时间包.秒)
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			执行集, 错误 := 智能体仓储包.新建仓储(运行时.数据库.数据库)
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			会话集, 错误 := 会话仓储包.新建仓储(运行时.数据库.数据库)
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			引擎, 错误 := 智能体服务包.新建(执行集, 会话集, 运行时.管理器, 运行时.事件集.发布, 运行时.日志记录器,
				智能体服务包.选项集{工具集: 执行器, 审批集: 运行时.获取审批集(), 工作区: 运行时.获取工作区集().获取,
					调用超时: 启动配置.调用超时})
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			快照 := 模型包.快照{标识: 策略测试标识(测试器), 显示名称: "测试模型", 远程模型标识: "test", 供应商: 模型包.供应商快照{标识: 策略测试标识(测试器), 名称: "测试厂商"}}
			服务, 错误 := 会话服务包.新建(会话集, func(上下文包.上下文, string) (智能体包.已解析, error) {
				return 智能体包.已解析{客户端: 策略测试模型{参数: JSON包.原始JSON消息(测试.参数), 已拒绝: 测试.动作 == 策略包.拒绝}, 快照: 快照, 上下文窗口: 1000000, 协议: "chatcompletions"}, nil
			}, 运行时.管理器, 引擎, 运行时.附件集, 运行时.技能, 运行时.事件集.发布, 运行时.日志记录器)
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			工作区, 错误 := 运行时.获取工作区集().创建(测试器.上下文(), "测试工作区", 测试器.创建临时目录())
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			订阅, 错误 := 运行时.订阅(事件集包.筛选{})
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			defer 订阅.关闭()
			var 已受理 会话包.执行
			错误 = 服务.发送(测试器.上下文(), 会话包.发送{模型标识: 快照.标识, 工作区标识: &工作区.标识, 流: new(false), 内容: []模型包.内容块{{类型: "text", 文本: "执行操作"}}}, func(本次执行 会话包.执行) error {
				已受理 = 本次执行
				if 测试.名称 == "always_read" {
					// 调用方修改回调中的快照，不能改变服务内部已经受理的权限。
					本次执行.执行.设置快照.修订号 = 0
					本次执行.执行.设置快照.审批策略 = 策略包.从不
				}
				return nil
			})
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			执行上下文, 取消函数 := 上下文包.创建超时上下文(测试器.上下文(), 5*时间包.秒)
			defer 取消函数()
			审批集 := 0
		循环:
			for {
				事件, 错误 := 订阅.下一个(执行上下文)
				if 错误 != nil {
					测试器.报告致命错误(错误)
				}
				switch 事件.类型 {
				case "approval.requested":
					if 测试.动作 != 策略包.询问 || 实例.调用集.加载() != 0 {
						测试器.报告致命错误("出现不应有的审批，或审批前已执行")
					}
					审批集++
					值 := 事件.数据.(智能体包.审批)
					if _, 错误 := 运行时.获取审批集().决定(测试器.上下文(), 值.标识, 智能体包.审批已批准, nil); 错误 != nil {
						测试器.报告致命错误(错误)
					}
				case "run.completed":
					break 循环
				case "run.failed", "run.cancelled":
					测试器.格式化报告致命错误("执行未完成: %+v", 事件)
				}
			}
			运行时.管理器.等待(已受理.执行.标识)
			预期调用集 := int32(1)
			if 测试.动作 == 策略包.拒绝 {
				预期调用集 = 0
			}
			if 实例.调用集.加载() != 预期调用集 {
				测试器.格式化报告致命错误("实际工具执行次数: %d，期望 %d", 实例.调用集.加载(), 预期调用集)
			}
			行集, 错误 := 运行时.获取审批集().列表(测试器.上下文(), 智能体包.审批筛选{执行标识: 已受理.执行.标识})
			if 错误 != nil || len(行集) != 审批集 || (测试.动作 == 策略包.询问 && 审批集 != 1) {
				测试器.格式化报告致命错误("审批记录数量不符: %+v %v", 行集, 错误)
			}
			步骤集, 错误 := 引擎.步骤集(测试器.上下文(), 已受理.执行.标识, 0, 20)
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			已找到 := false
			for _, 步骤 := range 步骤集 {
				if 步骤.类型 != "tool_call" {
					continue
				}
				已找到 = true
				var 输入 struct {
					策略  策略包.审批策略 `json:"approval_policy"`
					审核方 策略包.审核方  `json:"approvals_reviewer"`
					授权  策略包.决定   `json:"authorization"`
				}
				if 错误 := JSON包.反序列化(步骤.输入, &输入); 错误 != nil {
					测试器.报告致命错误(错误)
				}
				if 输入.策略 != 测试.策略 || 输入.审核方 != 策略包.用户 || 输入.授权.动作 != 测试.动作 || 输入.授权.原因 != 测试.预期原因 {
					测试器.格式化报告致命错误("审批判定审计不完整: %s", 步骤.输入)
				}
				if 测试.动作 == 策略包.拒绝 && (步骤.状态 != 智能体包.失败 || 步骤.开始时间 != nil) {
					测试器.报告致命错误("被拒绝的操作出现执行标记")
				}
			}
			if !已找到 {
				测试器.报告致命错误("缺少工具步骤")
			}
		})
	}
}

func 策略测试标识(测试器 *测试包.测试器) string {
	测试器.标记测试辅助函数()
	标识, 错误 := UUID包.新建UUIDv7()
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	return 标识.字符串()
}

func 设置运行时策略(测试器 *测试包.测试器, 运行时 *运行时, 选项 策略包.审批策略) 设置包.记录 {
	测试器.标记测试辅助函数()
	当前, 错误 := 运行时.获取设置().获取(测试器.上下文(), 设置包.执行键)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	原始, 错误 := JSON包.序列化(设置包.执行{审批策略: 选项, 审批审核方: 策略包.用户})
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	已保存, 错误 := 运行时.获取设置().更新(测试器.上下文(), 设置包.执行键, 当前.修订号, 原始)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	return 已保存
}

func Test无效审批设置保留修订号(测试器 *测试包.测试器) {
	运行时, _ := 运行时测试夹具(测试器, 1)
	当前, 错误 := 运行时.获取设置().获取(测试器.上下文(), 设置包.执行键)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	for _, 原始 := range []string{
		`{}`, `{"approval_policy":"never"}`, `{"approval_policy":"invalid","approvals_reviewer":"user"}`,
		`{"approval_policy":"never","approvals_reviewer":"auto_review"}`,
		`{"approval_policy":"always","approvals_reviewer":"user","sandbox_mode":"danger-full-access"}`,
		`{"approval_policy":"on-request","approvals_reviewer":"user","api_key":"forbidden-test-value"}`,
	} {
		if _, 错误 := 运行时.获取设置().更新(测试器.上下文(), 设置包.执行键, 当前.修订号, JSON包.原始JSON消息(原始)); !错误判断包.是否属于(错误, 设置包.无效错误) {
			测试器.格式化报告致命错误("未拒绝非法设置: %v", 错误)
		}
	}
	已保存, 错误 := 运行时.获取设置().获取(测试器.上下文(), 设置包.执行键)
	if 错误 != nil || 已保存.修订号 != 当前.修订号 || !字节包.相等(已保存.值, 当前.值) {
		测试器.格式化报告致命错误("非法请求修改了持久化设置: %+v %v", 已保存, 错误)
	}
}
```
