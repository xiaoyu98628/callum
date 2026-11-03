package schema

// Workspace 映射 workspaces（工作区表），软删除不级联修改会话状态。
type Workspace struct {
	ID         string `gorm:"type:text;primaryKey;not null;check:ck_workspace_id,id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-','')) = 32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'"`
	Name       string `gorm:"type:text;not null;check:ck_workspace_name,length(trim(name)) > 0"`
	RootPath   string `gorm:"type:text;not null;uniqueIndex:uq_workspace_path;check:ck_workspace_path,length(root_path) > 0"`
	PinnedAt   *int64
	ArchivedAt *int64
	DeletedAt  *int64 `gorm:"index:ix_workspace_listing,priority:1"`
	CreatedAt  int64  `gorm:"not null;autoCreateTime:false"`
	UpdatedAt  int64  `gorm:"not null;autoUpdateTime:false;index:ix_workspace_listing,priority:2,sort:desc"`
}

func (Workspace) TableName() string { return "workspaces" }
