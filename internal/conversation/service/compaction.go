package service

import (
	"context"
	"strings"

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
	lookup, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(lease.Context(), cancel)
	defer func() { stop(); cancel() }()
	resolved, err := s.resolve(lookup, in.ModelID)
	if err != nil {
		return err
	}
	if resolved.Client == nil || resolved.Snapshot.ID != in.ModelID || strings.TrimSpace(resolved.Snapshot.RemoteModelID) == "" {
		return conversation.ErrInvalid
	}
	ex, err := s.repository.BeginCompaction(lookup, conversation.Begin{ConversationID: in.ConversationID, RunID: runID, StepID: stepID, Model: resolved.Snapshot, Protocol: resolved.Protocol})
	if err != nil {
		return err
	}
	copy := ex
	if ex.Run.SettingsSnapshot != nil {
		value := ex.Run.SettingsSnapshot.Clone()
		copy.Run.SettingsSnapshot = &value
	}
	if ex.Context != nil {
		value := ex.Context.Clone()
		copy.Context = &value
	}
	if err := accepted(copy); err != nil {
		s.engine.Abandon(lease.Context(), ex, context.Canceled)
		return err
	}
	lease.Start(func(ctx context.Context) { s.engine.ExecuteCompaction(ctx, lease, ex, resolved) })
	return nil
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
