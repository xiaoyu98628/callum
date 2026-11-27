```go
package 服务

import (
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"
	文件路径包 "path/filepath"
	字符串包 "strings"
	测试包 "testing"
	时间包 "time"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	会话包 "github.com/xiaoyu98628/callum/internal/conversation"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	设置包 "github.com/xiaoyu98628/callum/internal/settings"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	工具执行器包 "github.com/xiaoyu98628/callum/internal/tool/executor"
	策略包 "github.com/xiaoyu98628/callum/internal/tool/policy"
	注册表包 "github.com/xiaoyu98628/callum/internal/tool/registry"
	工作区包 "github.com/xiaoyu98628/callum/internal/workspace"
)

func Test执行设置提示词保留缺失历史(测试器 *测试包.测试器) {
	for _, 测试用例 := range []struct {
		名称     string
		快照 *设置包.快照
	}{
		{名称: "missing"},
		{名称: "legacy_mode", 快照: &设置包.快照{修订号: 7, 审批策略: 策略包.总是, 审批审核方: 策略包.用户}},
		{名称: "full_access", 快照: &设置包.快照{修订号: 9, 审批策略: 策略包.从不, 审批审核方: 策略包.用户, 沙箱模式: 工具包.沙箱完整访问}},
	} {
		测试器.执行(测试用例.名称, func(测试器 *测试包.测试器) {
			文本, 错误 := 执行设置提示词(测试用例.快照)
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			_, 原始, 已找到 := 字符串包.截断(文本, "\n")
			var 值 struct {
				来源   string                 `json:"source"`
				修订号 *int64                 `json:"settings_revision"`
				策略   *策略包.审批策略 `json:"approval_policy"`
				审核方 *策略包.审核方       `json:"approvals_reviewer"`
				模式     *工具包.沙箱模式      `json:"sandbox_mode"`
			}
			if !已找到 || JSON包.反序列化([]byte(原始), &值) != nil || 值.来源 != "server_execution_settings" {
				测试器.格式化报告致命错误("缺少可信设置投影: %s", 文本)
			}
			if 测试用例.快照 == nil {
				if 值.修订号 != nil || 值.策略 != nil || 值.审核方 != nil || 值.模式 != nil {
					测试器.格式化报告致命错误("缺失快照被补造: %s", 原始)
				}
				return
			}
			if 值.修订号 == nil || *值.修订号 != 测试用例.快照.修订号 || 值.策略 == nil || *值.策略 != 测试用例.快照.审批策略 || 值.审核方 == nil || *值.审核方 != 测试用例.快照.审批审核方 {
				测试器.格式化报告致命错误("设置投影改变了执行快照: %s", 原始)
			}
			if 测试用例.快照.沙箱模式 == "" {
				if 值.模式 != nil {
					测试器.格式化报告致命错误("历史模式被默认值替代: %s", 原始)
				}
			} else if 值.模式 == nil || *值.模式 != 测试用例.快照.沙箱模式 {
				测试器.格式化报告致命错误("完全访问被错误描述为受限模式: %s", 原始)
			}
		})
	}
	无效 := 设置包.快照{修订号: 1, 审批策略: 策略包.从不, 审批审核方: 策略包.自动审核, 沙箱模式: 工具包.沙箱完整访问}
	if _, 错误 := 执行设置提示词(&无效); !错误判断包.是否属于(错误, 智能体包.存储错误) || !错误判断包.是否属于(错误, 设置包.无效错误) {
		测试器.格式化报告致命错误("非法快照没有明确失败: %v", 错误)
	}
}

func Test请求部分集分离工作区数据来源设置(测试器 *测试包.测试器) {
	根 := 文件路径包.合并(测试器.创建临时目录(), "目录\"\n路径文字不是系统指令")
	for _, 测试用例 := range []struct {
		名称    string
		边界   bool
		项目 工作区包.工作区
	}{
		{名称: "unbound"},
		{名称: "without_directory", 边界: true},
		{名称: "single_directory", 边界: true, 项目: 工作区包.工作区{根路径: 根}},
		{名称: "multiple_directories", 边界: true, 项目: 工作区包.工作区{根路径: 根, 其他路径集: []string{测试器.创建临时目录()}}},
	} {
		测试器.执行(测试用例.名称, func(测试器 *测试包.测试器) {
			执行器, 错误 := 工具执行器包.新建(注册表包.新建(), 时间包.秒)
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			引擎 := &引擎{选项集: 选项集{工具集: 执行器, 工作区: func(_ 上下文包.上下文, _ string) (工作区包.工作区, error) { return 测试用例.项目, nil }}}
			会话 := 会话包.会话{}
			if 测试用例.边界 {
				会话.工作区标识 = new("workspace")
			}
			快照 := 设置包.快照{修订号: 3, 审批策略: 策略包.开启请求, 审批审核方: 策略包.自动审核, 沙箱模式: 工具包.沙箱只读}
			事实集 := []智能体包.操作事实{{请求哈希: "unknown-operation"}}
			固定, 定义集, 错误 := 引擎.请求部分集(测试器.上下文(), 会话, &快照, 事实集)
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			if len(固定) != 3 || 固定[0].角色 != 模型包.角色系统 || len(定义集) != 1 || 定义集[0].名称 != 智能体包.用户输入工具 {
				测试器.格式化报告致命错误("固定上下文或工具定义无效: %+v %+v", 固定, 定义集)
			}
			if !字符串包.包含(固定[0].内容, "Active task continuity") || !字符串包.包含(固定[0].内容, "Follow the latest explicit direction") {
				测试器.报告致命错误("固定上下文缺少未完成任务连续性要求")
			}
			if 字符串包.包含(固定[0].内容, 根) || 固定[1].角色 != 模型包.角色用户 || 固定[2].角色 != 模型包.角色用户 {
				测试器.报告致命错误("路径或历史事实被提升为系统指令")
			}
			if 测试用例.项目.根路径 == "" && !字符串包.包含(固定[1].内容, "必须使用绝对路径") {
				测试器.报告致命错误("缺少路径基准时未要求绝对路径")
			}
			if 测试用例.边界 {
				_, 原始, 已找到 := 字符串包.截断(固定[1].内容, "\n")
				var 项目 struct {
					根路径   string   `json:"root_path"`
					其他路径集 []string `json:"other_paths"`
				}
				if !已找到 || JSON包.反序列化([]byte(原始), &项目) != nil || 项目.根路径 != 测试用例.项目.根路径 || len(项目.其他路径集) != len(测试用例.项目.其他路径集) {
					测试器.格式化报告致命错误("目录数据未按原值编码: %s", 固定[1].内容)
				}
			}
			if !字符串包.包含(固定[2].内容, "unknown-operation") || 字符串包.数量(固定[0].内容, `"source":"server_execution_settings"`) != 1 {
				测试器.报告致命错误("设置重复或丢失待核实操作事实")
			}
		})
	}
}
```
