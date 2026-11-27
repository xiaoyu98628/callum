```go
package 迁移集

import (
	错误判断包 "errors"
	切片包 "slices"

	GORM包 "gorm.io/gorm"
)

// 可选工作区根 保留原记录、索引和触发器，将无目录项目表示为 空 主目录。
// 调用方已持有实例锁、固定连接、关闭外键并开启事务；任何失败都回滚原表及版本。
func 可选工作区根(事务 *GORM包.数据库) error {
	var 列集 []string
	if 错误 := 事务.原始("SELECT name FROM pragma_table_info('workspaces') ORDER BY cid").扫描(&列集).错误; 错误 != nil {
		return 错误
	}
	if !切片包.相等(列集, []string{"id", "name", "root_path", "pinned_at", "archived_at", "deleted_at", "created_at", "updated_at"}) {
		return 错误判断包.新建("工作区字段与可升级结构不匹配，保留原数据库")
	}
	const 创建 = `CREATE TABLE workspaces_v2 (
id TEXT NOT NULL PRIMARY KEY CONSTRAINT ck_workspace_id CHECK(id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-','')) = 32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'),
name TEXT NOT NULL CONSTRAINT ck_workspace_name CHECK(length(trim(name)) > 0),
root_path TEXT CONSTRAINT ck_workspace_path CHECK(length(root_path) > 0),
pinned_at INTEGER, archived_at INTEGER, deleted_at INTEGER,
created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
) STRICT`
	return 重建表(事务, "workspaces", "升级后外键校验失败，保留原数据库",
		创建,
		"INSERT INTO workspaces_v2 SELECT id,name,root_path,pinned_at,archived_at,deleted_at,created_at,updated_at FROM workspaces",
		"DROP TABLE workspaces",
		"ALTER TABLE workspaces_v2 RENAME TO workspaces",
	)
}
```
