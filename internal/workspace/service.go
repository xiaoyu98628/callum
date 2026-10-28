package workspace

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Store 提供业务所需的持久化操作，由存储模块实现。
type Store interface {
	// Create 插入记录；路径已经存在时返回原记录，不修改其名称和状态。
	Create(context.Context, Workspace) (Workspace, error)
	List(context.Context, ListOptions) ([]Workspace, error)
}

// Service 校验工作空间参数并协调持久化，不持有数据库的关闭责任。
type Service struct {
	store  Store
	logger *slog.Logger
}

// NewService 使用应用注入的存储和日志装配工作空间服务。
func NewService(store Store, logger *slog.Logger) (*Service, error) {
	if store == nil || logger == nil {
		return nil, errors.New("工作空间存储和日志不能为空")
	}
	return &Service{store: store, logger: logger.With("component", "workspace")}, nil
}

// Create 登记已有目录；不创建、修改项目目录，不改变进程工作目录。
func (s *Service) Create(ctx context.Context, input CreateInput) (Workspace, error) {
	if err := checkContext(ctx); err != nil {
		return Workspace{}, err
	}
	if !filepath.IsAbs(input.RootPath) {
		return Workspace{}, fmt.Errorf("%w: root_path 必须是绝对路径", ErrInvalidInput)
	}
	root, err := filepath.EvalSymlinks(filepath.Clean(input.RootPath))
	if err != nil {
		return Workspace{}, fmt.Errorf("%w: 无法解析项目目录: %w", ErrInvalidInput, err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return Workspace{}, fmt.Errorf("%w: 无法读取项目目录: %w", ErrInvalidInput, err)
	}
	if !info.IsDir() {
		return Workspace{}, fmt.Errorf("%w: root_path 必须指向目录", ErrInvalidInput)
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = filepath.Base(root)
	}
	now := time.Now().UnixMilli()
	candidate := Workspace{ID: rand.Text(), Name: name, RootPath: root, CreatedAt: now, UpdatedAt: now}
	result, err := s.store.Create(ctx, candidate)
	if err != nil {
		return Workspace{}, fmt.Errorf("保存工作空间失败: %w", err)
	}
	if result.ID == candidate.ID {
		s.logger.InfoContext(ctx, "工作空间已创建", "workspace_id", result.ID, "root_path", result.RootPath)
	} else {
		s.logger.DebugContext(ctx, "返回已有工作空间", "workspace_id", result.ID)
	}
	return result, nil
}

// List 返回存储层已经按置顶和更新时间排序的记录，空列表编码为 []。
func (s *Service) List(ctx context.Context, options ListOptions) ([]Workspace, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	items, err := s.store.List(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("查询工作空间失败: %w", err)
	}
	if items == nil {
		items = []Workspace{}
	}
	s.logger.DebugContext(ctx, "工作空间列表已查询", "include_archived", options.IncludeArchived, "count", len(items))
	return items, nil
}

func checkContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("工作空间上下文不能为空")
	}
	return ctx.Err()
}
