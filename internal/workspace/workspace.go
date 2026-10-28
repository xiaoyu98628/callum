package workspace

import "errors"

// ErrInvalidInput 表示调用方提供的工作空间参数无效。
var ErrInvalidInput = errors.New("工作空间参数无效")

// Workspace 是持久化的工作空间记录，时间字段使用 Unix 毫秒。
type Workspace struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	RootPath   string `json:"root_path"`
	PinnedAt   *int64 `json:"pinned_at"`
	ArchivedAt *int64 `json:"archived_at"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
}

// CreateInput 登记已存在的项目目录，Name 为空时使用目录名称。
type CreateInput struct {
	RootPath string `json:"root_path"`
	Name     string `json:"name,omitempty"`
}

// ListOptions 控制列表是否包含已经归档的记录。
type ListOptions struct {
	IncludeArchived bool `json:"include_archived,omitempty"`
}
