package service

import (
	"context"

	"github.com/xiaoyu98628/callum/internal/conversation"
)

// Compact 受理独立的历史压缩，复用会话互斥、全局容量和连接所属执行管理。
func (s *Service) Compact(ctx context.Context, in conversation.Compact, accepted func(conversation.Execution) error) error {
	if !validID(in.ConversationID) || (in.ModelID != "" && !validID(in.ModelID)) || accepted == nil {
		return conversation.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if in.ModelID == "" {
		value, err := s.repository.Get(ctx, in.ConversationID)
		if err != nil {
			return err
		}
		in.ModelID = value.Model.ID
	}
	runID, err := newID()
	if err != nil {
		return err
	}
	stepID, err := newID()
	if err != nil {
		return err
	}
	lease, err := s.manager.ReserveAgentContext(ctx, runID, in.ConversationID)
	if err != nil {
		return err
	}
	defer lease.Abort()
	lookup, cancel := acceptanceContext(ctx, lease)
	defer cancel()
	resolved, err := s.resolveModel(lookup, in.ModelID)
	if err != nil {
		return err
	}
	ex, err := s.repository.BeginCompaction(lookup, conversation.Begin{ConversationID: in.ConversationID, RunID: runID, StepID: stepID, Model: resolved.Snapshot, Protocol: resolved.Protocol})
	if err != nil {
		return err
	}
	return s.start(lease, ex, accepted, func(ctx context.Context) {
		s.engine.ExecuteCompaction(ctx, lease, ex, resolved)
	})
}

func (s *Service) ContextVersions(ctx context.Context, id string, limit, offset int) ([]conversation.ContextVersion, error) {
	if limit == 0 {
		limit = 50
	}
	if !validID(id) || limit < 1 || limit > 200 || offset < 0 {
		return nil, conversation.ErrInvalid
	}
	return s.repository.ContextVersions(ctx, id, limit, offset)
}

func (s *Service) ContextVersion(ctx context.Context, conversationID, id string) (conversation.ContextVersion, error) {
	if !validID(conversationID) || !validID(id) {
		return conversation.ContextVersion{}, conversation.ErrInvalid
	}
	return s.repository.ContextVersion(ctx, conversationID, id)
}
