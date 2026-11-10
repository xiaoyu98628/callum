package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	"gorm.io/gorm"
)

const schemaVersion = 4

// Run 在同一个事务内初始化空库或校验当前结构。调用方持有连接与实例锁。
func Run(ctx context.Context, db *gorm.DB) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if db == nil {
		return errors.New("初始化需要数据库")
	}
	// 固定同一连接，确保迁移期间的外键设置只影响初始化连接且在返回前恢复。
	return db.WithContext(ctx).Connection(func(conn *gorm.DB) (err error) {
		conn = conn.Session(&gorm.Session{NewDB: true})
		var version int
		if err := conn.Raw("PRAGMA user_version").Scan(&version).Error; err != nil {
			return err
		}
		if version == 1 || version == 2 {
			// SQLite 重建父表必须在事务外临时关闭外键，避免触发子表级联删除。
			if err := conn.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
				return err
			}
			defer func() {
				err = errors.Join(err, conn.WithContext(context.WithoutCancel(ctx)).Exec("PRAGMA foreign_keys = ON").Error)
			}()
		}
		return runSchema(ctx, conn, version)
	})
}

func runSchema(ctx context.Context, db *gorm.DB, version int) (err error) {
	tx := db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("开始数据库初始化: %w", tx.Error)
	}
	defer func() {
		rollbackErr := tx.Rollback().Error
		if !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, rollbackErr)
		}
	}()
	// 外层读取只决定是否需关闭外键；取得写事务后重新读取，串行处理并发初始化。
	var actualVersion int
	if err := tx.Raw("PRAGMA user_version").Scan(&actualVersion).Error; err != nil {
		return err
	}
	if (actualVersion == 1 || actualVersion == 2) && version != actualVersion {
		return errors.New("数据库结构版本在初始化期间发生变化，请重新打开")
	}
	version = actualVersion
	if version > 0 && version <= 3 {
		if !tx.Migrator().HasColumn(&schema.Provider{}, "credential_format") {
			if err := tx.Exec("ALTER TABLE model_providers ADD COLUMN credential_format integer NOT NULL DEFAULT 0 CONSTRAINT ck_provider_credential_format CHECK (credential_format IN (0,1))").Error; err != nil {
				return err
			}
		}
	}
	tables := slices.Concat(catalogTables(), conversationTables(), approvalTables(), settingsTables())
	if version == 0 {
		if err := initialize(tx, tables); err != nil {
			return err
		}
	} else if version == 1 || version == 2 {
		if err := validateSchema(tx, tables); err != nil {
			return fmt.Errorf("数据库结构与版本 %d 不匹配: %w", version, err)
		}
		if version == 1 {
			if err := optionalWorkspaceRoot(tx); err != nil {
				return fmt.Errorf("升级工作区目录结构: %w", err)
			}
		}
		if err := allowAutoReviewer(tx); err != nil {
			return fmt.Errorf("升级审批人约束: %w", err)
		}
	} else if version != 3 && version != schemaVersion {
		return fmt.Errorf("不支持数据库结构版本 %d，请使用独立开发数据目录", version)
	}
	if err := validateSchema(tx, tables); err != nil {
		return fmt.Errorf("数据库结构与当前版本 %d 不匹配: %w", schemaVersion, err)
	}
	if err := validateAutoReviewer(tx); err != nil {
		return err
	}
	var required int
	if err := tx.Raw(`SELECT "notnull" FROM pragma_table_info('workspaces') WHERE name='root_path'`).Scan(&required).Error; err != nil {
		return err
	}
	if required != 0 {
		return errors.New("工作区主目录仍具有非空约束")
	}
	if version != schemaVersion {
		if err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)).Error; err != nil {
			return err
		}
	}
	return tx.Commit().Error
}

func initialize(tx *gorm.DB, tables []any) error {
	for _, table := range tables {
		if tx.Migrator().HasTable(table) {
			return errors.New("数据库已有业务表但缺少结构版本，无法安全初始化")
		}
	}
	if err := tx.Set("gorm:table_options", "STRICT").Migrator().CreateTable(tables...); err != nil {
		return fmt.Errorf("创建数据库结构: %w", err)
	}
	return initializeSettings(tx)
}

func validateSchema(tx *gorm.DB, tables []any) error {
	for _, table := range tables {
		if err := validateCoreTable(tx, table); err != nil {
			return err
		}
	}
	for _, validate := range []func(*gorm.DB) error{validateCatalog, validateWorkspaceFolders, validateContextIndex, validateApprovals, validateSettings} {
		if err := validate(tx); err != nil {
			return err
		}
	}
	return nil
}

func validateCoreTable(tx *gorm.DB, table any) error {
	statement := &gorm.Statement{DB: tx}
	if err := statement.Parse(table); err != nil {
		return err
	}
	var ddl string
	if err := tx.Raw("SELECT sql FROM sqlite_schema WHERE type='table' AND name=?", statement.Schema.Table).Scan(&ddl).Error; err != nil {
		return err
	}
	if !strings.HasSuffix(strings.ToUpper(strings.TrimSpace(ddl)), "STRICT") {
		return fmt.Errorf("%s 不是 STRICT 表", statement.Schema.Table)
	}
	// 直接使用 SQLite 的结构事实，避免 GORM 为每张表反复用正则解析大型 CHECK DDL。
	// 同时保留字段、类型、非空及单列主键的原有校验强度。
	type columnInfo struct {
		Name    string
		Type    string
		Notnull int
		Pk      int
	}
	var columns []columnInfo
	if err := tx.Raw("SELECT name,type,\"notnull\",pk FROM pragma_table_info(?)", statement.Schema.Table).Scan(&columns).Error; err != nil {
		return err
	}
	actual := make(map[string]columnInfo, len(columns))
	primary := 0
	for _, column := range columns {
		actual[column.Name] = column
		if column.Pk > 0 {
			primary++
		}
	}
	if primary != 1 {
		return errors.New("表必须具有单列主键")
	}
	for _, field := range statement.Schema.Fields {
		if field.DBName == "" {
			continue
		}
		column, exists := actual[field.DBName]
		if !exists {
			return fmt.Errorf("%s 缺少字段 %s", statement.Schema.Table, field.DBName)
		}
		want := field.TagSettings["TYPE"]
		if want == "" {
			want = "integer"
		}
		if !strings.EqualFold(column.Type, want) {
			return fmt.Errorf("%s.%s 类型不匹配", statement.Schema.Table, field.DBName)
		}
		if field.NotNull && column.Notnull == 0 {
			return fmt.Errorf("%s.%s 缺少非空约束", statement.Schema.Table, field.DBName)
		}
		if field.PrimaryKey {
			if column.Pk == 0 {
				return errors.New("表主键不匹配")
			}
		}
		if check := field.TagSettings["CHECK"]; check != "" {
			name, _, _ := strings.Cut(check, ",")
			if !tx.Migrator().HasConstraint(table, name) {
				return fmt.Errorf("缺少检查约束 %s", name)
			}
		}
	}
	for _, index := range statement.Schema.ParseIndexes() {
		if !tx.Migrator().HasIndex(table, index.Name) {
			return fmt.Errorf("缺少索引 %s", index.Name)
		}
	}
	for name := range statement.Schema.Relationships.Relations {
		if !tx.Migrator().HasConstraint(table, name) {
			return fmt.Errorf("缺少关联约束 %s", name)
		}
	}
	return nil
}

// 使用有界元数据查询校验真实唯一索引，避免仅凭索引名称接受损坏的同版本结构。
func validateUniqueIndex(tx *gorm.DB, table, name string, want []string) error {
	var info struct {
		Name    string
		Unique  int
		Partial int
	}
	if err := tx.Raw(`SELECT name,"unique",partial FROM pragma_index_list(?) WHERE name=?`, table, name).Scan(&info).Error; err != nil {
		return err
	}
	var columns []string
	if err := tx.Raw("SELECT name FROM pragma_index_info(?) ORDER BY seqno", name).Scan(&columns).Error; err != nil {
		return err
	}
	if info.Name != name || info.Unique != 1 || info.Partial != 0 || !slices.Equal(columns, want) {
		return errors.New("唯一索引结构不匹配")
	}
	return nil
}
