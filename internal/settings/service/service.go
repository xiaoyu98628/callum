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
	if key != settings.ExecutionKey && key != settings.CompactionKey {
		return settings.Record{}, settings.ErrInvalid
	}
	value, err := s.repository.Get(ctx, key)
	if err != nil {
		return value, err
	}
	if key == settings.ExecutionKey {
		snapshot, err := settings.ExecutionSnapshot(value)
		if err != nil {
			return settings.Record{}, err
		}
		// 将旧配置的明确兼容默认值返回给客户端，不改写历史记录和 revision。
		value.Value, err = json.Marshal(snapshot.Execution)
		if err != nil {
			return settings.Record{}, errors.Join(settings.ErrStorage, err)
		}
	} else if _, err := settings.DecodeCompactionSnapshot(value); err != nil {
		return settings.Record{}, err
	}
	return value, nil
}

// Update 完整替换一个已注册配置组；调用方提交其读取到的 revision，冲突后重新读取。
// 值在写入前完成校验，返回成功表示事务已经提交，后续受理执行即可读取新值。
func (s *Service) Update(ctx context.Context, key string, revision int64, raw json.RawMessage) (settings.Record, error) {
	if (key != settings.ExecutionKey && key != settings.CompactionKey) || revision < 1 || revision == math.MaxInt64 {
		return settings.Record{}, settings.ErrInvalid
	}
	var value any
	var err error
	if key == settings.ExecutionKey {
		var execution settings.Execution
		execution, err = settings.DecodeExecution(raw)
		if err == nil {
			var fields map[string]json.RawMessage
			if err = json.Unmarshal(raw, &fields); err == nil && fields["sandbox_mode"] == nil {
				// 旧客户端完整替换原有两字段时，保留用户已选的沙箱模式。
				var current settings.Record
				current, err = s.Get(ctx, key)
				if err == nil && current.Revision != revision {
					return settings.Record{}, settings.ErrConflict
				}
				if err == nil {
					var snapshot settings.Snapshot
					snapshot, err = settings.ExecutionSnapshot(current)
					execution.SandboxMode = snapshot.SandboxMode
				}
			}
		}
		if err == nil && !execution.Valid() {
			err = settings.ErrInvalid
		}
		value = execution
	} else {
		value, err = settings.DecodeCompaction(raw)
	}
	if err != nil {
		return settings.Record{}, err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return settings.Record{}, errors.Join(settings.ErrInvalid, err)
	}
	return s.repository.Update(ctx, key, revision, canonical)
}
