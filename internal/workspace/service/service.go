package service

import (
	"context"
	"path/filepath"
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

func (s *Service) Create(ctx context.Context, name, rootPath string) (workspace.Workspace, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 1024 || !filepath.IsAbs(rootPath) {
		return workspace.Workspace{}, workspace.ErrInvalid
	}
	id, err := uuid.NewV7()
	if err != nil {
		return workspace.Workspace{}, err
	}
	now := time.Now().UnixMilli()
	return s.repository.Create(ctx, workspace.Workspace{ID: id.String(), Name: name, RootPath: filepath.Clean(rootPath), CreatedAt: now, UpdatedAt: now})
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
	if change.Name != nil {
		value := strings.TrimSpace(*change.Name)
		if value == "" || len(value) > 1024 {
			return workspace.Workspace{}, workspace.ErrInvalid
		}
		change.Name = &value
	}
	return s.repository.Update(ctx, id, change)
}
