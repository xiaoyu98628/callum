package sqlite

import (
	"context"
	"errors"
	"fmt"

	"github.com/xiaoyu98628/callum/internal/workspace"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ORM 映射限定在存储层，业务结构不依赖 GORM，也不引入额外的模型字段。
type workspaceRecord struct {
	ID         string `gorm:"column:id;primaryKey"`
	Name       string `gorm:"column:name"`
	RootPath   string `gorm:"column:root_path"`
	PinnedAt   *int64 `gorm:"column:pinned_at"`
	ArchivedAt *int64 `gorm:"column:archived_at"`
	// 时间由业务层使用 Unix 毫秒维护，关闭 ORM 的自动时间赋值。
	CreatedAt int64 `gorm:"column:created_at;autoCreateTime:false"`
	UpdatedAt int64 `gorm:"column:updated_at;autoUpdateTime:false"`
}

func (workspaceRecord) TableName() string { return "workspaces" }

// WorkspaceStore 通过 ORM 访问应用持有的数据库，不负责关闭连接池。
type WorkspaceStore struct {
	db *gorm.DB
}

// NewWorkspaceStore 复用已经初始化 ORM 并完成迁移的数据库。
func NewWorkspaceStore(db *gorm.DB) (*WorkspaceStore, error) {
	if db == nil {
		return nil, errors.New("工作空间数据库不能为空")
	}
	return &WorkspaceStore{db: db}, nil
}

// Create 由唯一约束协调跨进程的重复创建，已有记录保持不变。
func (s *WorkspaceStore) Create(ctx context.Context, candidate workspace.Workspace) (workspace.Workspace, error) {
	if ctx == nil {
		return workspace.Workspace{}, errors.New("工作空间上下文不能为空")
	}
	record := workspaceRecord(candidate)
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "root_path"}}, DoNothing: true,
	}).Create(&record)
	if result.Error != nil {
		return workspace.Workspace{}, fmt.Errorf("写入工作空间记录失败: %w", result.Error)
	}
	if result.RowsAffected != 0 {
		return workspace.Workspace(record), nil
	}
	// 路径冲突时使用空目标查询，防止 GORM 将候选记录的主键附加到查询条件。
	// 当前没有删除入口，插入和查询之间不会由本服务删除原记录。
	var existing workspaceRecord
	err := s.db.WithContext(ctx).Where(clause.Eq{Column: "root_path", Value: candidate.RootPath}).Take(&existing).Error
	if err != nil {
		return workspace.Workspace{}, fmt.Errorf("查询已有工作空间失败: %w", err)
	}
	return workspace.Workspace(existing), nil
}

// List 默认隐藏归档记录；置顶时间、更新时间倒序，ID 升序保证稳定顺序。
func (s *WorkspaceStore) List(ctx context.Context, options workspace.ListOptions) ([]workspace.Workspace, error) {
	if ctx == nil {
		return nil, errors.New("工作空间上下文不能为空")
	}
	query := s.db.WithContext(ctx)
	if !options.IncludeArchived {
		query = query.Where(clause.Eq{Column: "archived_at", Value: nil})
	}
	var records []workspaceRecord
	err := query.Order(clause.OrderByColumn{Column: clause.Column{Name: "pinned_at"}, Desc: true}).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "updated_at"}, Desc: true}).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "id"}}).Find(&records).Error
	if err != nil {
		return nil, fmt.Errorf("读取工作空间列表失败: %w", err)
	}
	items := make([]workspace.Workspace, len(records))
	for index, record := range records {
		items[index] = workspace.Workspace(record)
	}
	return items, nil
}
