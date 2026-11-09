package workspace

// Workspace 是用户任务操作的项目目录，不是应用数据目录。
type Workspace struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	RootPath   string   `json:"root_path"`
	OtherPaths []string `json:"other_paths"`
	PinnedAt   *int64   `json:"pinned_at"`
	ArchivedAt *int64   `json:"archived_at"`
	DeletedAt  *int64   `json:"deleted_at"`
	CreatedAt  int64    `json:"created_at"`
	UpdatedAt  int64    `json:"updated_at"`
}

// Change 只更新明确提供的字段；RootPath 只能选择已有源目录，false 清除对应状态时间。
type Change struct {
	Name     *string `json:"name,omitempty"`
	RootPath *string `json:"root_path,omitempty"`
	Pinned   *bool   `json:"pinned,omitempty"`
	Archived *bool   `json:"archived,omitempty"`
	Deleted  *bool   `json:"-"`
}

// Filter 是有界列表查询，Archived、Deleted 选择对应状态的记录。
type Filter struct {
	Archived bool `json:"archived"`
	Deleted  bool `json:"deleted"`
	Limit    int  `json:"limit"`
	Offset   int  `json:"offset"`
}
