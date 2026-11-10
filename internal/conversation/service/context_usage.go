package service

import (
	"context"
	"unicode/utf8"

	"github.com/xiaoyu98628/callum/internal/conversation"
)

// ContextUsage 返回当前模型下的只读上下文预览；草稿不会保存或发送到模型。
func (s *Service) ContextUsage(ctx context.Context, in conversation.ContextUsageRequest) (conversation.ContextUsage, error) {
	if err := ctx.Err(); err != nil {
		return conversation.ContextUsage{}, err
	}
	if (in.ConversationID != "" && !validID(in.ConversationID)) || (in.ModelID != "" && !validID(in.ModelID)) || (in.WorkspaceID != nil && (!validID(*in.WorkspaceID) || in.ConversationID != "")) || len(in.Draft) > 1<<20 || !utf8.ValidString(in.Draft) {
		return conversation.ContextUsage{}, conversation.ErrInvalid
	}
	if in.ConversationID != "" {
		if value, ok, err := s.engine.LiveContextUsage(in); ok || err != nil {
			return value, err
		}
	}
	state, err := s.repository.ContextState(ctx, in.ConversationID)
	if err != nil {
		return conversation.ContextUsage{}, err
	}
	if state.Settings.Compaction == nil || !state.Settings.Compaction.Valid() {
		return conversation.ContextUsage{}, conversation.ErrInvalid
	}
	if in.ModelID == "" {
		in.ModelID = state.Conversation.Model.ID
	}
	if !validID(in.ModelID) {
		return conversation.ContextUsage{}, conversation.ErrInvalid
	}
	if state.ActiveRun != nil && state.ActiveRun.Model.ID != in.ModelID {
		return conversation.ContextUsage{}, conversation.ErrBusy
	}
	resolved, err := s.resolve(ctx, in.ModelID)
	if err != nil {
		return conversation.ContextUsage{}, err
	}
	if resolved.Snapshot.ID != in.ModelID || resolved.Client == nil {
		return conversation.ContextUsage{}, conversation.ErrInvalid
	}
	return s.engine.PreviewContext(ctx, in, state, resolved)
}
