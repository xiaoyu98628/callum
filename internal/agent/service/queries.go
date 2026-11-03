package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/xiaoyu98628/callum/internal/agent"
)

func validID(id string) bool {
	value, err := uuid.Parse(id)
	return err == nil && value.Version() == 7 && value.String() == id
}

// Get 查询持久化执行，不把旧 model.invoke 的临时标识当作数据库主键。
func (e *Engine) Get(ctx context.Context, id string) (agent.Run, error) {
	if !validID(id) {
		return agent.Run{}, agent.ErrInvalid
	}
	return e.repository.Get(ctx, id)
}

// List 返回会话的有界执行历史。
func (e *Engine) List(ctx context.Context, id string, limit, offset int) ([]agent.Run, error) {
	if !validID(id) || limit < 1 || limit > 200 || offset < 0 {
		return nil, agent.ErrInvalid
	}
	return e.repository.List(ctx, id, limit, offset)
}

// GetStep 返回完整步骤，通信层按需要投影元数据或分段原始数据。
func (e *Engine) GetStep(ctx context.Context, id string) (agent.Step, error) {
	if !validID(id) {
		return agent.Step{}, agent.ErrInvalid
	}
	return e.repository.GetStep(ctx, id)
}

// Steps 按执行内序号读取步骤，不向通信层暴露写入仓储。
func (e *Engine) Steps(ctx context.Context, id string, after int64, limit int) ([]agent.Step, error) {
	if !validID(id) || after < 0 || limit < 1 || limit > 200 {
		return nil, agent.ErrInvalid
	}
	return e.repository.Steps(ctx, id, after, limit)
}
