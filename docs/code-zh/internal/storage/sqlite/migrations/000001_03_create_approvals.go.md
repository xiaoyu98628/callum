```go
package 迁移集

import (
	错误判断包 "errors"

	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	GORM包 "gorm.io/gorm"
)

func 审批表集() []any {
	return []any{&表结构包.审批{}, &表结构包.目录授权{}}
}

func 校验审批集(事务 *GORM包.数据库) error {
	// 组合外键需要步骤表中精确匹配的唯一索引。
	if 错误 := 校验唯一索引(事务, "conversation_run_steps", "uq_step_run", []string{"id", "run_id"}); 错误 != nil {
		return 错误
	}
	列集, 错误 := 事务.迁移器().列类型集(&表结构包.审批{})
	if 错误 != nil {
		return 错误
	}
	语句 := &GORM包.语句{数据库: 事务}
	if 错误 := 语句.解析(&表结构包.审批{}); 错误 != nil {
		return 错误
	}
	if len(列集) != len(语句.表结构.数据库名称集) {
		return 错误判断包.新建("审批表字段与当前结构不匹配")
	}
	if 错误 := 校验唯一索引(事务, "conversation_run_approvals", "uq_approval_step", []string{"step_id"}); 错误 != nil {
		return 错误
	}
	return 校验唯一索引(事务, "conversation_directory_grants", "uq_directory_grant_approval", []string{"approval_id"})
}
```
