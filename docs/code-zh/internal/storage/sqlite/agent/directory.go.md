```go
package 智能体包

import (
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"
	时间包 "time"

	UUID包 "github.com/google/uuid"
	GORM包 "gorm.io/gorm"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	策略包 "github.com/xiaoyu98628/callum/internal/tool/policy"
)

type 目录请求 struct {
	工具 工具包.定义  `json:"tool"`
	目标 *工具包.目标 `json:"target"`
	计划 *工具包.计划 `json:"plan"`
}

func (当前目录请求 目录请求) 目标集() []工具包.目标 {
	var 目标集 []工具包.目标
	if 当前目录请求.目标 != nil {
		目标集 = append(目标集, *当前目录请求.目标)
	}
	if 当前目录请求.计划 != nil {
		目标集 = append(目标集, 当前目录请求.计划.目标集...)
	}
	return 目标集
}

func 目录授权(行 表结构包.目录授权) 智能体包.目录授权 {
	return 智能体包.目录授权{标识: 行.标识, 会话标识: 行.会话标识, 审批标识: 行.审批标识, 范围: 行.范围, 创建时间: 行.创建时间, 撤销时间: 行.撤销时间}
}

// 审批详情 从固定步骤读取审核投影并推导目录授权选项。
func 审批详情(事务 *GORM包.数据库, 当前审批 表结构包.审批, 输出 *智能体包.审批) error {
	var 行 表结构包.步骤
	if 错误 := 事务.选择("input").条件("id=? AND run_id=?", 当前审批.步骤标识, 当前审批.执行标识).取出(&行).错误; 错误 != nil {
		return 错误
	}
	var 输入 struct {
		策略 策略包.审批策略   `json:"approval_policy"`
		授权 策略包.决定     `json:"authorization"`
		审核 *智能体包.审批审核 `json:"approval_review"`
	}
	var 请求 目录请求
	if JSON包.反序列化(行.输入, &输入) != nil || JSON包.反序列化(当前审批.请求, &请求) != nil {
		return 智能体包.审批无效错误
	}
	输出.审核 = 输入.审核
	if 输出.状态 != 智能体包.审批待处理 && 输出.已决定依据 != nil && *输出.已决定依据 == 智能体包.审批自动审核方 {
		return nil
	}
	if 输入.策略 != 策略包.开启请求 || 输入.授权.动作 != 策略包.询问 || 输入.授权.原因 != "outside_workspace" {
		return nil
	}
	输出.目录范围 = 策略包.目录选项(请求.目标集(), 请求.工具.影响集)
	return nil
}

func 装饰审批(事务 *GORM包.数据库, 行 表结构包.审批) (智能体包.审批, error) {
	输出 := 审批(行)
	if 错误 := 审批详情(事务, 行, &输出); 错误 != nil {
		return 输出, 错误
	}
	var 授权 表结构包.目录授权
	错误 := 事务.条件("approval_id=?", 行.标识).取出(&授权).错误
	if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
		return 输出, nil
	}
	if 错误 == nil {
		输出.决定范围 = 智能体包.审批会话目录
		输出.目录授权 = new(目录授权(授权))
	}
	return 输出, 错误
}

func 保存目录授权(事务 *GORM包.数据库, 行 表结构包.审批, 范围 *工具包.目录范围) error {
	if 范围 == nil {
		return 智能体包.审批无效错误
	}
	var 运行 表结构包.执行
	if 错误 := 事务.条件("id=?", 行.执行标识).取出(&运行).错误; 错误 != nil {
		return 错误
	}
	if 运行.设置快照 == nil || !运行.设置快照.有效() || 运行.设置快照.审批策略 != 策略包.开启请求 {
		return 智能体包.审批无效错误
	}
	标识, 错误 := UUID包.新建UUIDv7()
	if 错误 != nil {
		return 错误
	}
	授权 := 表结构包.目录授权{标识: 标识.字符串(), 会话标识: 运行.会话标识, 审批标识: 行.标识, 范围: *范围, 创建时间: 时间包.当前时间().Unix毫秒()}
	return 事务.省略("Conversation", "Approval").创建(&授权).错误
}

func (当前仓储 *仓储) 列表目录授权集(执行上下文 上下文包.上下文, 会话标识 string, 上限, 偏移 int) ([]智能体包.目录授权, error) {
	var 行集 []表结构包.目录授权
	if 错误 := 当前仓储.数据库.带有上下文(执行上下文).条件("conversation_id=?", 会话标识).顺序("created_at DESC").顺序("id DESC").上限(上限).偏移(偏移).查找(&行集).错误; 错误 != nil {
		return nil, 错误
	}
	输出 := make([]智能体包.目录授权, 0, len(行集))
	for _, 行 := range 行集 {
		输出 = append(输出, 目录授权(行))
	}
	return 输出, nil
}

// 撤销目录授权 与工具认领共用事务边界，已经开始的操作不被宣称为撤销。
func (当前仓储 *仓储) 撤销目录授权(执行上下文 上下文包.上下文, 会话标识, 标识 string) (输出 智能体包.目录授权, 错误 error) {
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		var 行 表结构包.目录授权
		if 错误 := 事务.条件("id=? AND conversation_id=?", 标识, 会话标识).取出(&行).错误; 错误 != nil {
			return 未找到(错误)
		}
		if 行.撤销时间 == nil {
			行.撤销时间 = new(时间包.当前时间().Unix毫秒())
			if 错误 := 事务.模型(&表结构包.目录授权{}).条件("id=? AND revoked_at IS NULL", 标识).更新("revoked_at", 行.撤销时间).错误; 错误 != nil {
				return 错误
			}
		}
		输出 = 目录授权(行)
		return nil
	})
	return
}

func 检查目录认领(事务 *GORM包.数据库, 行 表结构包.步骤) error {
	var 输入 struct {
		标识集 []string `json:"directory_grant_ids"`
		策略  策略包.审批策略 `json:"approval_policy"`
		授权  策略包.决定   `json:"authorization"`
		请求  目录请求     `json:"request"`
	}
	if JSON包.反序列化(行.输入, &输入) != nil {
		return 工具包.已拒绝错误
	}
	if len(输入.标识集) == 0 {
		if 输入.授权.原因 == "conversation_directory_grant" {
			return 工具包.已拒绝错误
		}
		return nil
	}
	if 输入.策略 != 策略包.开启请求 || 输入.授权.动作 != 策略包.允许 || 输入.授权.原因 != "conversation_directory_grant" {
		return 工具包.已拒绝错误
	}
	var 授权集 []表结构包.目录授权
	if 错误 := 事务.条件("id IN ? AND conversation_id=? AND revoked_at IS NULL", 输入.标识集, 行.会话标识).查找(&授权集).错误; 错误 != nil {
		return 错误
	}
	范围集 := make([]工具包.目录范围, 0, len(授权集))
	for _, 授权 := range 授权集 {
		范围集 = append(范围集, 授权.范围)
	}
	if !策略包.目录集覆盖(范围集, 输入.请求.目标集(), 输入.请求.工具.影响集) {
		return 工具包.已拒绝错误
	}
	return nil
}
```
