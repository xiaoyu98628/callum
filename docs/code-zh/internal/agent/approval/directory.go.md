```go
package 审批

import (
	上下文包 "context"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	策略包 "github.com/xiaoyu98628/callum/internal/tool/policy"
)

// 列表目录集 按会话分页读取授权及其撤销记录，权限读取失败不回退为放行。
func (当前服务 *服务) 列表目录集(执行上下文 上下文包.上下文, 会话标识 string, 上限, 偏移 int) ([]智能体包.目录授权, error) {
	if 上限 == 0 {
		上限 = 100
	}
	if !有效标识(会话标识) || 上限 < 1 || 上限 > 200 || 偏移 < 0 {
		return nil, 智能体包.审批无效错误
	}
	return 当前服务.仓储.列表目录授权集(执行上下文, 会话标识, 上限, 偏移)
}

// 撤销目录 保存撤销后发布业务事件；来源审批与已认领的操作保持原义。
func (当前服务 *服务) 撤销目录(执行上下文 上下文包.上下文, 会话标识, 标识 string) (智能体包.目录授权, error) {
	if !有效标识(会话标识) || !有效标识(标识) {
		return 智能体包.目录授权{}, 智能体包.审批无效错误
	}
	值, 错误 := 当前服务.仓储.撤销目录授权(执行上下文, 会话标识, 标识)
	if 错误 != nil {
		return 值, 错误
	}
	return 值, 当前服务.发出事件(智能体包.事件{类型: "directory_grant.updated", 会话标识: 会话标识, 数据: 值})
}

// 匹配目录集 每次查询当前会话授权，不缓存撤销状态；认领工具时由仓储在事务内再次核验。
func (当前服务 *服务) 匹配目录集(执行上下文 上下文包.上下文, 会话标识 string, 目标集 []工具包.目标, 影响集 []工具包.影响) ([]string, error) {
	已覆盖 := make([]bool, len(目标集))
	剩余 := 0
	for 索引, 目标 := range 目标集 {
		已覆盖[索引] = 目标.范围内工作区
		if !已覆盖[索引] {
			剩余++
		}
	}
	if 剩余 == 0 {
		return nil, nil
	}
	var 标识集 []string
	已使用 := map[string]bool{}
	for 偏移 := 0; ; 偏移 += 200 {
		if 错误 := 执行上下文.获取错误(); 错误 != nil {
			return nil, 错误
		}
		页, 错误 := 当前服务.列表目录集(执行上下文, 会话标识, 200, 偏移)
		if 错误 != nil {
			return nil, 错误
		}
		for _, 授权 := range 页 {
			if 授权.撤销时间 != nil {
				continue
			}
			for 索引, 目标 := range 目标集 {
				if !已覆盖[索引] && 策略包.目录覆盖(授权.范围, 目标, 影响集) {
					已覆盖[索引] = true
					剩余--
					if !已使用[授权.标识] {
						标识集 = append(标识集, 授权.标识)
						已使用[授权.标识] = true
					}
				}
			}
		}
		if 剩余 == 0 {
			return 标识集, nil
		}
		if len(页) < 200 {
			break
		}
	}
	return nil, nil
}
```
