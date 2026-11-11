package service

import (
	"context"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/conversation/history"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/tokens"
)

// PreviewContext 分批恢复同一消息边界的有效历史，只估算、不压缩、不调用模型。
func (e *Engine) PreviewContext(ctx context.Context, in conversation.ContextUsageRequest, state conversation.ContextState, resolved agent.Resolved) (conversation.ContextUsage, error) {
	value := usageBase(in.ConversationID, "", resolved, state.Settings.Compaction.Compaction)
	value.Revision, value.UpdatedAt = e.ContextRevision(), time.Now().UnixMilli()
	if state.ActiveRun != nil {
		value.RunID = state.ActiveRun.ID
		value.InputLimit, value.ContextWindow, value.Reason = nil, nil, "context_preparing"
		return value, nil
	}
	conv := state.Conversation
	if in.ConversationID == "" {
		conv.WorkspaceID = in.WorkspaceID
	}
	var facts []agent.OperationFact
	var err error
	if in.ConversationID != "" {
		facts, err = e.repository.OperationFacts(ctx, in.ConversationID)
		if err != nil {
			return value, operationStorageError(ctx, err)
		}
	}
	fixed, definitions, err := e.requestParts(ctx, conv, facts)
	if err != nil {
		return value, err
	}
	if state.Context != nil {
		fixed = append(fixed, contextMessages(state.Context, nil)...)
	}
	total := tokens.Estimate(fixed, definitions, state.Settings.Compaction.ImageTokenEstimate)
	after := int64(0)
	if state.Context != nil {
		after = state.Context.ThroughSequence
	}
	if in.ConversationID != "" {
		for {
			if err := ctx.Err(); err != nil {
				return value, err
			}
			rows, more, err := e.conversations.ContextPage(ctx, in.ConversationID, after, state.ThroughSequence+1)
			if err != nil {
				return previewFailure(value, err)
			}
			if len(rows) == 0 {
				if more {
					return value, model.ErrInvalidResponse
				}
				break
			}
			last := rows[len(rows)-1].Sequence
			if last <= after {
				return value, model.ErrInvalidResponse
			}
			traces, err := e.repository.ContextTranscript(ctx, in.ConversationID, after, last+1)
			if err != nil {
				return previewFailure(value, err)
			}
			messages, err := history.Assemble(rows, traces)
			if err != nil {
				return previewFailure(value, err)
			}
			total += tokens.Estimate(messages, nil, state.Settings.Compaction.ImageTokenEstimate) - 16
			after = last
			if !more {
				break
			}
		}
		// 读取期间可能受理新执行、编辑或切换摘要，不能返回混合版本的占用。
		latest, err := e.conversations.ContextState(ctx, in.ConversationID)
		if err != nil {
			return value, err
		}
		if live, ok, err := e.LiveContextUsage(in); ok || err != nil {
			return live, err
		}
		if !sameContextState(state, latest) {
			value.State, value.Reason = "refresh_required", "context_changed"
			return value, nil
		}
	}
	value.State, value.EstimatedTokens = "ready", new(total)
	if state.ActualModelID == resolved.Snapshot.ID {
		value.ActualInputTokens = copyInt(state.ActualInputTokens)
	}
	return withDraft(value, in.Draft, in.DraftImages, state.Settings.Compaction.ImageTokenEstimate), nil
}

func previewFailure(value conversation.ContextUsage, err error) (conversation.ContextUsage, error) {
	if capacityError(err) {
		value.State, value.Reason, value.EstimatedTokens = "unavailable", "history_limit", nil
		return value, nil
	}
	return value, err
}

func sameContextState(a, b conversation.ContextState) bool {
	if b.ActiveRun != nil || a.ThroughSequence != b.ThroughSequence || a.LastUserRunID != b.LastUserRunID || a.Conversation.UpdatedAt != b.Conversation.UpdatedAt || a.Settings.Revision != b.Settings.Revision || a.Settings.Compaction.Revision != b.Settings.Compaction.Revision {
		return false
	}
	if (a.Context == nil) != (b.Context == nil) {
		return false
	}
	return a.Context == nil || a.Context.ID == b.Context.ID
}
