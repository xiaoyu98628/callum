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

// Change 只更新明确提供的字段；完整目录更新携带原目录集合，避免覆盖并发修改。
type Change struct {
	Name            *string   `json:"name,omitempty"`
	RootPath        *string   `json:"root_path,omitempty"`
	OtherPaths      *[]string `json:"other_paths,omitempty"`
	ExpectedFolders *[]string `json:"expected_folders,omitempty"`
	Pinned          *bool     `json:"pinned,omitempty"`
	Archived        *bool     `json:"archived,omitempty"`
	Deleted         *bool     `json:"-"`
}

// Filter 是有界列表查询，Archived、Deleted 选择对应状态的记录。
type Filter struct {
	Archived bool `json:"archived"`
	Deleted  bool `json:"deleted"`
	Limit    int  `json:"limit"`
	Offset   int  `json:"offset"`
}
