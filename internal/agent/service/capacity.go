package service

import (
	"context"
	"errors"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/conversation/history"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/tokens"
)

func capacityError(err error) bool {
	return errors.Is(err, model.ErrInputLimit) || errors.Is(err, model.ErrTokenLimit)
}

// inputCapacity 统一检查 Token、消息数量和适配器实际编码大小，使用同一执行设置快照。
func inputCapacity(ctx context.Context, ex conversation.Execution, resolved agent.Resolved, messages []model.Message, definitions []model.ToolDefinition, meter *tokens.Meter) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := history.Check(messages); err != nil {
		return err
	}
	if snapshot := ex.Run.SettingsSnapshot; snapshot != nil && snapshot.Compaction != nil && snapshot.Compaction.Enabled {
		cfg := snapshot.Compaction
		budget, err := cfg.InputBudget(resolved.ContextWindow, resolved.MaxOutputTokens)
		if err != nil {
			return model.ErrTokenBudget
		}
		estimate := tokens.Estimate(messages, definitions, cfg.ImageTokenEstimate)
		if meter != nil {
			estimate = meter.Estimate(messages, definitions, cfg.ImageTokenEstimate).Tokens
		}
		if estimate > budget {
			return model.ErrTokenLimit
		}
	}
	if validator, ok := resolved.Client.(model.InputValidator); ok {
		return validator.ValidateInput(ctx, model.Request{Model: resolved.Snapshot.RemoteModelID, AllowImageInput: resolved.ImageInput, Messages: messages, Tools: definitions})
	}
	return nil
}

func autoCompaction(ex conversation.Execution) bool {
	return ex.Run.SettingsSnapshot != nil && ex.Run.SettingsSnapshot.Compaction != nil && ex.Run.SettingsSnapshot.Compaction.Enabled
}

// summaryCapacity 手动压缩也必须具备有效预算，不随自动压缩开关关闭该约束。
func summaryCapacity(ctx context.Context, ex conversation.Execution, resolved agent.Resolved, messages []model.Message) error {
	if ex.Run.SettingsSnapshot == nil || ex.Run.SettingsSnapshot.Compaction == nil {
		return model.ErrTokenBudget
	}
	cfg := ex.Run.SettingsSnapshot.Compaction
	budget, err := cfg.InputBudget(resolved.ContextWindow, resolved.MaxOutputTokens)
	if err != nil {
		return model.ErrTokenBudget
	}
	if tokens.Estimate(messages, nil, cfg.ImageTokenEstimate) > budget {
		return model.ErrTokenLimit
	}
	return inputCapacity(ctx, ex, resolved, messages, nil, nil)
}
