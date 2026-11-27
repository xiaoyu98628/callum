```go
// 包 设置 定义应用设置、执行快照及持久化契约。
package 设置

import (
	字节包 "bytes"
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"
	输入输出包 "io"
	UTF8包 "unicode/utf8"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	策略包 "github.com/xiaoyu98628/callum/internal/tool/policy"
)

const 执行键 = "execution"
const 最大值字节 = 16 << 10

var (
	无效错误  = 错误判断包.新建("应用设置无效")
	未找到错误 = 错误判断包.新建("应用设置不存在")
	冲突错误  = 错误判断包.新建("应用设置版本已变化")
	存储错误  = 错误判断包.新建("应用设置存储失败")
)

// 记录 保存一个配置组；值 不包含凭据，不能直接接受任意未注册配置。
type 记录 struct {
	标识   string         `json:"id"`
	键    string         `json:"key"`
	值    JSON包.原始JSON消息 `json:"value"`
	修订号  int64          `json:"revision"`
	创建时间 int64          `json:"created_at"`
	更新时间 int64          `json:"updated_at"`
}

// 执行 是当前支持的执行设置，新增 JSON 字段时同步定义默认值和校验。
type 执行 struct {
	审批策略  策略包.审批策略 `json:"approval_policy"`
	审批审核方 策略包.审核方  `json:"approvals_reviewer"`
	沙箱模式  工具包.沙箱模式 `json:"sandbox_mode,omitempty"`
}

// 默认执行 只在创建设置记录时使用，读取失败不能回退到默认权限。
func 默认执行() 执行 {
	return 执行{审批策略: 策略包.开启请求, 审批审核方: 策略包.用户, 沙箱模式: 工具包.沙箱工作区写入}
}

// 有效 校验审批人与沙箱的组合。旧快照可保留未记录的沙箱模式。
func (当前执行 执行) 有效() bool {
	if !当前执行.审批策略.有效() || !当前执行.审批审核方.有效() || (当前执行.沙箱模式 != "" && !当前执行.沙箱模式.有效()) {
		return false
	}
	if 当前执行.沙箱模式 == 工具包.沙箱完整访问 {
		return 当前执行.审批策略 == 策略包.从不 && 当前执行.审批审核方 == 策略包.用户
	}
	return 当前执行.审批审核方 != 策略包.自动审核 || (当前执行.沙箱模式 != "" && 当前执行.审批策略 != 策略包.从不)
}

// 快照 固定一次执行实际采用的配置及版本，不能跟随全局设置更新。
type 快照 struct {
	修订号 int64 `json:"revision"`
	执行
	压缩 *压缩快照 `json:"context_compaction,omitempty"`
	// MCP修订号 固定执行受理时的 MCP 设置版本，历史记录缺失时为零。
	MCP修订号 int64 `json:"mcp_revision,omitempty"`
}

// 有效 判断快照是否具有有效版本和当前服务支持的独立配置值。
func (当前快照 快照) 有效() bool {
	return 当前快照.修订号 > 0 && 当前快照.MCP修订号 >= 0 && 当前快照.执行.有效() &&
		(当前快照.压缩 == nil || 当前快照.压缩.有效())
}

// 复制 复制可变嵌套快照，受理回调不得修改即将执行的配置。
func (当前快照 快照) 复制() 快照 {
	if 当前快照.压缩 != nil {
		值 := *当前快照.压缩
		当前快照.压缩 = &值
	}
	return 当前快照
}

// 解码执行 严格校验客户端或存储中的 JSON；不接受重复键、未知字段或尾随内容。
func 解码执行(原始 JSON包.原始JSON消息) (执行, error) {
	// 旧全局设置缺少此字段时，新受理执行采用工作区沙箱；历史快照直接解码，保持空值。
	值 := 执行{沙箱模式: 工具包.沙箱工作区写入}
	if len(原始) == 0 || len(原始) > 最大值字节 || !UTF8包.有效(原始) {
		return 值, 无效错误
	}
	解码器 := JSON包.新建解码器(字节包.新建读取器(原始))
	启动, 错误 := 解码器.词元()
	if 错误 != nil || 启动 != JSON包.分隔符('{') {
		return 值, 无效错误
	}
	已见 := map[string]bool{}
	for 解码器.更多() {
		词元, 错误 := 解码器.词元()
		键, 成功 := 词元.(string)
		if 错误 != nil || !成功 || 已见[键] {
			return 值, 无效错误
		}
		已见[键] = true
		switch 键 {
		case "approval_policy":
			错误 = 解码器.解码(&值.审批策略)
		case "approvals_reviewer":
			错误 = 解码器.解码(&值.审批审核方)
		case "sandbox_mode":
			var 已选择 *工具包.沙箱模式
			错误 = 解码器.解码(&已选择)
			if 已选择 == nil {
				return 值, 无效错误
			}
			值.沙箱模式 = *已选择
		default:
			return 值, 无效错误
		}
		if 错误 != nil {
			return 值, 无效错误
		}
	}
	if _, 错误 := 解码器.词元(); 错误 != nil {
		return 值, 无效错误
	}
	var 尾部 any
	if 解码器.解码(&尾部) != 输入输出包.文件结束 || !值.有效() || !值.沙箱模式.有效() {
		return 值, 无效错误
	}
	return 值, nil
}

// 执行快照 用于受理事务和查询验证；非法存储不能被当作默认设置。
func 执行快照(记录 记录) (快照, error) {
	值, 错误 := 解码执行(记录.值)
	if 错误 != nil || 记录.键 != 执行键 || 记录.修订号 <= 0 {
		return 快照{}, 错误判断包.合并(存储错误, 无效错误)
	}
	return 快照{修订号: 记录.修订号, 执行: 值}, nil
}

// 仓储 的 更新 必须以期望版本原子更新，冲突时不得改动设置。
// 获取 不启动事务，允许仓储使用受理动作传入的同一事务读取配置。
type 仓储 interface {
	获取(上下文包.上下文, string) (记录, error)
	更新(上下文包.上下文, string, int64, JSON包.原始JSON消息) (记录, error)
}
```
