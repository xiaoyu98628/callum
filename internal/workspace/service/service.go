package service

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/xiaoyu98628/callum/internal/workspace"
)

// Service 校验工作区操作；数据库所有权属于装配层。
type Service struct{ repository workspace.Repository }

func New(repository workspace.Repository) (*Service, error) {
	if repository == nil {
		return nil, workspace.ErrInvalid
	}
	return &Service{repository: repository}, nil
}

// Create 保存项目。省略其他目录时，重新添加已删除的项目会保留原目录。
func (s *Service) Create(ctx context.Context, name, rootPath string, otherPaths ...string) (workspace.Workspace, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 1024 || (rootPath != "" && !validPath(rootPath)) || len(otherPaths) > 15 || (rootPath == "" && len(otherPaths) != 0) {
		return workspace.Workspace{}, workspace.ErrInvalid
	}
	if rootPath != "" {
		rootPath = filepath.Clean(rootPath)
	}
	if len(otherPaths) > 0 {
		actualRoot, rootInfo, err := realDirectory(rootPath)
		if err != nil {
			return workspace.Workspace{}, err
		}
		rootPath = actualRoot
		seen := []os.FileInfo{rootInfo}
		normalized := make([]string, 0, len(otherPaths))
		for _, path := range otherPaths {
			if err := ctx.Err(); err != nil {
				return workspace.Workspace{}, err
			}
			actual, info, err := realDirectory(path)
			if err != nil || slices.ContainsFunc(seen, func(previous os.FileInfo) bool { return os.SameFile(previous, info) }) {
				return workspace.Workspace{}, workspace.ErrInvalid
			}
			seen = append(seen, info)
			normalized = append(normalized, actual)
		}
		otherPaths = normalized
	}
	id, err := uuid.NewV7()
	if err != nil {
		return workspace.Workspace{}, err
	}
	now := time.Now().UnixMilli()
	return s.repository.Create(ctx, workspace.Workspace{ID: id.String(), Name: name, RootPath: rootPath, OtherPaths: otherPaths, CreatedAt: now, UpdatedAt: now})
}

func validPath(path string) bool {
	return len(path) <= 8192 && !strings.ContainsRune(path, 0) && filepath.IsAbs(path)
}

func realDirectory(path string) (string, os.FileInfo, error) {
	if !validPath(path) {
		return "", nil, workspace.ErrInvalid
	}
	actual, err := filepath.EvalSymlinks(path)
	if err != nil || !validPath(actual) {
		return "", nil, workspace.ErrInvalid
	}
	info, err := os.Stat(actual)
	if err != nil || !info.IsDir() {
		return "", nil, workspace.ErrInvalid
	}
	return actual, info, nil
}

func validID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed.Version() == 7 && parsed.String() == id
}

func (s *Service) Get(ctx context.Context, id string) (workspace.Workspace, error) {
	if !validID(id) {
		return workspace.Workspace{}, workspace.ErrInvalid
	}
	return s.repository.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, filter workspace.Filter) ([]workspace.Workspace, error) {
	if filter.Limit == 0 {
		filter.Limit = 100
	}
	if filter.Limit < 1 || filter.Limit > 200 || filter.Offset < 0 {
		return nil, workspace.ErrInvalid
	}
	return s.repository.List(ctx, filter)
}

func (s *Service) Update(ctx context.Context, id string, change workspace.Change) (workspace.Workspace, error) {
	if !validID(id) {
		return workspace.Workspace{}, workspace.ErrInvalid
	}
	if (change.OtherPaths == nil) != (change.ExpectedFolders == nil) {
		return workspace.Workspace{}, workspace.ErrInvalid
	}
	if change.Name != nil {
		value := strings.TrimSpace(*change.Name)
		if value == "" || len(value) > 1024 {
			return workspace.Workspace{}, workspace.ErrInvalid
		}
		change.Name = &value
	}
	if change.OtherPaths != nil {
		if change.RootPath == nil || change.ExpectedFolders == nil || len(*change.OtherPaths) > 15 || len(*change.ExpectedFolders) > 16 {
			return workspace.Workspace{}, workspace.ErrInvalid
		}
		for _, path := range *change.ExpectedFolders {
			if !validPath(path) {
				return workspace.Workspace{}, workspace.ErrInvalid
			}
		}
		root, others, err := normalizeFolders(ctx, *change.RootPath, *change.OtherPaths)
		if err != nil {
			return workspace.Workspace{}, err
		}
		change.RootPath, change.OtherPaths = &root, &others
	} else if change.RootPath != nil {
		path, _, err := realDirectory(*change.RootPath)
		if err != nil {
			return workspace.Workspace{}, err
		}
		change.RootPath = &path
	}
	return s.repository.Update(ctx, id, change)
}

// normalizeFolders 允许无目录项目；绑定目录时按实际身份拒绝重复和符号链接别名。
func normalizeFolders(ctx context.Context, root string, others []string) (string, []string, error) {
	if root == "" {
		if len(others) != 0 {
			return "", nil, workspace.ErrInvalid
		}
		return "", []string{}, nil
	}
	paths := append([]string{root}, others...)
	seen := make([]os.FileInfo, 0, len(paths))
	for i, path := range paths {
		if err := ctx.Err(); err != nil {
			return "", nil, err
		}
		actual, info, err := realDirectory(path)
		if err != nil || slices.ContainsFunc(seen, func(previous os.FileInfo) bool { return os.SameFile(previous, info) }) {
			return "", nil, workspace.ErrInvalid
		}
		paths[i] = actual
		seen = append(seen, info)
	}
	return paths[0], paths[1:], nil
}
