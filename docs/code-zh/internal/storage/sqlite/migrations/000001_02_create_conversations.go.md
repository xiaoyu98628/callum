```go
package 迁移集

import (
	错误判断包 "errors"
	字符串包 "strings"

	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	GORM包 "gorm.io/gorm"
)

// 项目目录、会话及执行相关表按外键依赖排列。
func 会话表集() []any {
	return []any{&表结构包.工作区{}, &表结构包.工作区附加文件夹{}, &表结构包.会话{}, &表结构包.执行{}, &表结构包.消息{}, &表结构包.步骤{}, &表结构包.上下文版本{}}
}

func 校验工作区文件夹集(事务 *GORM包.数据库) error {
	return 校验唯一索引(事务, "workspace_additional_folders", "uq_workspace_additional_folder", []string{"workspace_id", "path"})
}

func 校验上下文索引(事务 *GORM包.数据库) error {
	if 错误 := 校验唯一索引(事务, "conversation_context_versions", "uq_context_version", []string{"conversation_id", "version"}); 错误 != nil {
		return 错误
	}
	var 建表语句 string
	if 错误 := 事务.原始("SELECT sql FROM sqlite_schema WHERE type='index' AND name='uq_context_active' AND tbl_name='conversation_context_versions'").扫描(&建表语句).错误; 错误 != nil {
		return 错误
	}
	if !字符串包.包含(建表语句, "UNIQUE INDEX") || !字符串包.存在后缀(建表语句, "(`conversation_id`) WHERE status = 'active'") {
		return 错误判断包.新建("有效上下文唯一索引不匹配")
	}
	return nil
}
```
