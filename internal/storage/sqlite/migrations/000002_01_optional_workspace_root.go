package migrations

import (
	"errors"
	"slices"

	"gorm.io/gorm"
)

// optionalWorkspaceRoot 保留原记录、索引和触发器，将无目录项目表示为 NULL 主目录。
// 调用方已持有实例锁、固定连接、关闭外键并开启事务；任何失败都回滚原表及版本。
func optionalWorkspaceRoot(tx *gorm.DB) error {
	var columns []string
	if err := tx.Raw("SELECT name FROM pragma_table_info('workspaces') ORDER BY cid").Scan(&columns).Error; err != nil {
		return err
	}
	if !slices.Equal(columns, []string{"id", "name", "root_path", "pinned_at", "archived_at", "deleted_at", "created_at", "updated_at"}) {
		return errors.New("工作区字段与可升级结构不匹配，保留原数据库")
	}
	var statements []string
	if err := tx.Raw("SELECT sql FROM sqlite_schema WHERE tbl_name='workspaces' AND type IN ('index','trigger') AND sql IS NOT NULL ORDER BY type,name").Scan(&statements).Error; err != nil {
		return err
	}
	const create = `CREATE TABLE workspaces_v2 (
id TEXT NOT NULL PRIMARY KEY CONSTRAINT ck_workspace_id CHECK(id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-','')) = 32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'),
name TEXT NOT NULL CONSTRAINT ck_workspace_name CHECK(length(trim(name)) > 0),
root_path TEXT CONSTRAINT ck_workspace_path CHECK(length(root_path) > 0),
pinned_at INTEGER, archived_at INTEGER, deleted_at INTEGER,
created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
) STRICT`
	for _, query := range []string{
		create,
		"INSERT INTO workspaces_v2 SELECT id,name,root_path,pinned_at,archived_at,deleted_at,created_at,updated_at FROM workspaces",
		"DROP TABLE workspaces",
		"ALTER TABLE workspaces_v2 RENAME TO workspaces",
	} {
		if err := tx.Exec(query).Error; err != nil {
			return err
		}
	}
	for _, statement := range statements {
		if err := tx.Exec(statement).Error; err != nil {
			return err
		}
	}
	var invalid int
	if err := tx.Raw("SELECT count(*) FROM (SELECT 1 FROM pragma_foreign_key_check LIMIT 1)").Scan(&invalid).Error; err != nil {
		return err
	}
	if invalid != 0 {
		return errors.New("升级后外键校验失败，保留原数据库")
	}
	return nil
}
