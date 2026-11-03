// Package service 校验已注册的应用设置，不依赖通信协议或全局内存缓存。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"

	"github.com/xiaoyu98628/callum/internal/settings"
)

// Service 借用设置仓储，数据库生命周期由装配层负责。
type Service struct{ repository settings.Repository }

func New(repository settings.Repository) (*Service, error) {
	if repository == nil {
		return nil, settings.ErrInvalid
	}
	return &Service{repository: repository}, nil
}

func (s *Service) Get(ctx context.Context, key string) (settings.Record, error) {
	if key != settings.ExecutionKey {
		return settings.Record{}, settings.ErrInvalid
	}
	value, err := s.repository.Get(ctx, key)
	if err != nil {
		return value, err
	}
	if _, err := settings.ExecutionSnapshot(value); err != nil {
		return settings.Record{}, err
	}
	return value, nil
}

// Update 完整替换一个已注册配置组；调用方提交其读取到的 revision，冲突后重新读取。
// 值在写入前完成校验，返回成功表示事务已经提交，后续受理执行即可读取新值。
func (s *Service) Update(ctx context.Context, key string, revision int64, raw json.RawMessage) (settings.Record, error) {
	if key != settings.ExecutionKey || revision < 1 || revision == math.MaxInt64 {
		return settings.Record{}, settings.ErrInvalid
	}
	value, err := settings.DecodeExecution(raw)
	if err != nil {
		return settings.Record{}, err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return settings.Record{}, errors.Join(settings.ErrInvalid, err)
	}
	return s.repository.Update(ctx, key, revision, canonical)
}
