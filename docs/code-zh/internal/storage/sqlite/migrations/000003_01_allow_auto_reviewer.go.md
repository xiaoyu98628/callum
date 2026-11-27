```go
package 迁移集

import (
	错误判断包 "errors"
	字符串包 "strings"

	GORM包 "gorm.io/gorm"
)

const 旧版审核方检查 = "json_extract(settings_snapshot,'$.approvals_reviewer') IS 'user'"
const 自动审核方检查 = "COALESCE(json_extract(settings_snapshot,'$.approvals_reviewer') IN ('user','auto_review'),0)"

func 执行表建表语句(事务 *GORM包.数据库) (string, error) {
	var 建表语句 string
	错误 := 事务.原始("SELECT sql FROM sqlite_schema WHERE type='table' AND name='conversation_runs'").扫描(&建表语句).错误
	return 建表语句, 错误
}

func 校验自动审核方(事务 *GORM包.数据库) error {
	建表语句, 错误 := 执行表建表语句(事务)
	if 错误 != nil {
		return 错误
	}
	if 字符串包.数量(建表语句, 自动审核方检查) != 1 {
		return 错误判断包.新建("执行快照审批人约束与当前版本不匹配")
	}
	return nil
}

// 允许自动审核方 只替换审批人的 检查 表达式。调用方固定连接并关闭外键，事务失败时恢复整张表。
func 允许自动审核方(事务 *GORM包.数据库) error {
	建表语句, 错误 := 执行表建表语句(事务)
	if 错误 != nil {
		return 错误
	}
	if 字符串包.包含(建表语句, 自动审核方检查) {
		return nil
	}
	if 字符串包.数量(建表语句, 旧版审核方检查) != 1 {
		return 错误判断包.新建("未找到可升级的执行快照审批人约束")
	}
	建表语句 = 字符串包.替换(建表语句, 旧版审核方检查, 自动审核方检查, 1)
	建表语句 = 字符串包.替换(建表语句, "conversation_runs", "conversation_runs_v3", 1)
	return 重建表(事务, "conversation_runs", "审批人约束升级后外键校验失败",
		建表语句,
		"INSERT INTO conversation_runs_v3 SELECT * FROM conversation_runs",
		"DROP TABLE conversation_runs",
		"ALTER TABLE conversation_runs_v3 RENAME TO conversation_runs",
	)
}
```
