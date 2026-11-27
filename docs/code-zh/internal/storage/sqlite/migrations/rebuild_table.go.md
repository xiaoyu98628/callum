```go
package 迁移集

import (
	错误判断包 "errors"
	切片包 "slices"

	GORM包 "gorm.io/gorm"
)

// 重建表 在调用方的事务中重建表、恢复索引及触发器，再校验外键。
// 调用方在固定连接上临时关闭外键，负责回滚失败的迁移。
func 重建表(事务 *GORM包.数据库, 表, 外键键失败 string, 查询集 ...string) error {
	var 语句集 []string
	if 错误 := 事务.原始("SELECT sql FROM sqlite_schema WHERE tbl_name=? AND type IN ('index','trigger') AND sql IS NOT NULL ORDER BY type,name", 表).扫描(&语句集).错误; 错误 != nil {
		return 错误
	}
	for _, 语句 := range 切片包.拼接(查询集, 语句集) {
		if 错误 := 事务.进程执行(语句).错误; 错误 != nil {
			return 错误
		}
	}
	var 无效 int
	if 错误 := 事务.原始("SELECT count(*) FROM (SELECT 1 FROM pragma_foreign_key_check LIMIT 1)").扫描(&无效).错误; 错误 != nil {
		return 错误
	}
	if 无效 != 0 {
		return 错误判断包.新建(外键键失败)
	}
	return nil
}
```
