package service

import (
	"context"
	"errors"
	"strings"

	"github.com/xiaoyu98628/callum/internal/agent/executor"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/model"
)

func (s *Service) FollowUps(ctx context.Context, id string) ([]conversation.FollowUp, error) {
	if !validID(id) {
		return nil, conversation.ErrInvalid
	}
	return s.repository.FollowUps(ctx, id)
}

func (s *Service) AddFollowUp(ctx context.Context, in conversation.FollowUpInput) (out conversation.FollowUp, err error) {
	if !validID(in.ConversationID) || !validID(in.ExpectedRunID) || (in.Mode != "queue" && in.Mode != "steer") {
		return out, conversation.ErrInvalid
	}
	var lease *executor.Lease
	if in.Mode == "steer" {
		lease = s.engine.SteeringLease(in.ExpectedRunID)
		if lease == nil {
			return out, conversation.ErrFollowUpChanged
		}
	}
	content := append([]model.Block(nil), in.Content...)
	if err := validate(conversation.Send{ConversationID: in.ConversationID, ModelID: in.ModelID, Content: content}); err != nil {
		return out, err
	}
	if err := s.validateSkills(ctx, content); err != nil {
		return out, err
	}
	conv, err := s.repository.Get(ctx, in.ConversationID)
	if err != nil {
		return out, err
	}
	if in.ModelID == "" {
		in.ModelID = conv.Model.ID
	}
	if in.Mode == "steer" {
		run, err := s.engine.Get(ctx, in.ExpectedRunID)
		if err != nil || run.ConversationID != in.ConversationID {
			return out, conversation.ErrFollowUpChanged
		}
		in.ModelID = run.Model.ID
	}
	resolved, err := s.resolveModel(ctx, in.ModelID)
	if err != nil {
		return out, err
	}
	id, err := newID()
	if err != nil {
		return out, err
	}
	batch, err := s.stageAttachments(ctx, in.ConversationID, id, content, resolved)
	if err != nil {
		return out, err
	}
	defer func() { batch.Finish(ctx, err == nil) }()
	status := conversation.FollowUpQueued
	if in.Mode == "steer" {
		status = conversation.FollowUpSteering
	}
	write := func() (conversation.FollowUp, error) {
		out, err = s.repository.CreateFollowUp(ctx, conversation.FollowUp{ID: id, ConversationID: in.ConversationID,
			ExpectedRunID: in.ExpectedRunID, ModelID: in.ModelID, Status: status, Content: content})
		return out, err
	}
	if in.Mode == "steer" {
		err = s.engine.AcceptSteering(lease, write)
	} else {
		_, err = write()
		if err == nil {
			s.followUpsUpdated(in.ConversationID, in.ExpectedRunID)
		}
	}
	return out, err
}

func (s *Service) ChangeFollowUp(ctx context.Context, in conversation.FollowUpChange) (out conversation.FollowUp, err error) {
	steer := in.Action == "steer" || in.Action == "steer_draft"
	queue := in.Action == "queue_draft"
	edit := in.Action == "edit" || in.Action == "steer_draft" || queue
	if !validID(in.ConversationID) || !validID(in.ID) || in.Revision < 1 ||
		((steer || queue) && in.ExpectedRunID == "") ||
		(in.ExpectedRunID != "" && (!(steer || queue || in.Action == "restore_draft") || !validID(in.ExpectedRunID))) ||
		edit != (in.Text != nil) || (in.ModelID != "" && (!queue || !validID(in.ModelID))) {
		return out, conversation.ErrInvalid
	}
	var lease *executor.Lease
	if steer {
		lease = s.engine.SteeringLease(in.ExpectedRunID)
		if lease == nil {
			return out, conversation.ErrFollowUpChanged
		}
	}
	item, err := s.repository.FollowUp(ctx, in.ConversationID, in.ID)
	if err != nil {
		return out, err
	}
	var content []model.Block
	if edit {
		content, err = editedFollowUpContent(item, *in.Text)
		if err != nil {
			return out, err
		}
	}
	if steer || queue {
		modelID := item.ModelID
		if steer {
			run, err := s.engine.Get(ctx, in.ExpectedRunID)
			if err != nil || run.ConversationID != in.ConversationID {
				return out, conversation.ErrFollowUpChanged
			}
			modelID = run.Model.ID
		} else if in.ModelID != "" {
			modelID = in.ModelID
		}
		resolved, err := s.resolveModel(ctx, modelID)
		if err != nil {
			return out, err
		}
		for _, block := range item.Content {
			if block.Type == "image" && !resolved.ImageInput {
				return out, inference.ErrUnsupported
			}
		}
	}
	write := func() (conversation.FollowUp, error) {
		out, err = s.repository.ChangeFollowUp(ctx, in, content)
		return out, err
	}
	if steer {
		err = s.engine.AcceptSteering(lease, write)
	} else {
		_, err = write()
		if err == nil {
			s.followUpsUpdated(in.ConversationID, in.ExpectedRunID)
		}
	}
	if err != nil {
		return out, err
	}
	if in.Action == "remove" {
		s.attachments.RemoveFollowUp(in.ConversationID, in.ID)
	}
	return out, nil
}

// 编辑保留服务端已经保管的附件与技能，客户端只能提交正文。
func editedFollowUpContent(item conversation.FollowUp, text string) ([]model.Block, error) {
	content := make([]model.Block, 0, len(item.Content)+1)
	if text = strings.TrimSpace(text); text != "" {
		content = append(content, model.Block{Type: "text", Text: text})
	}
	for _, block := range item.Content {
		if block.Type != "text" {
			content = append(content, block)
		}
	}
	return content, validate(conversation.Send{ConversationID: item.ConversationID, MessageID: item.ID, Content: content})
}

func (s *Service) StartFollowUp(ctx context.Context, in conversation.FollowUpStart, accepted func(conversation.Execution) error) error {
	if accepted == nil || !validID(in.ConversationID) || !validID(in.ID) || in.Revision < 1 ||
		(in.AfterRunID != "" && (!validID(in.AfterRunID) || in.Text != nil)) ||
		(in.ModelID != "" && (in.Text == nil || !validID(in.ModelID))) {
		return conversation.ErrInvalid
	}
	item, err := s.repository.FollowUp(ctx, in.ConversationID, in.ID)
	if err != nil {
		return err
	}
	status := conversation.FollowUpQueued
	if in.Text != nil {
		status = conversation.FollowUpDraft
	}
	if item.Status != status || item.Revision != in.Revision {
		return conversation.ErrFollowUpChanged
	}
	content, modelID := item.Content, item.ModelID
	if in.Text != nil {
		content, err = editedFollowUpContent(item, *in.Text)
		if err != nil {
			return err
		}
		if in.ModelID != "" {
			modelID = in.ModelID
		}
	}
	return s.send(ctx, conversation.Send{ConversationID: in.ConversationID, ModelID: modelID, Content: content}, &in, func(ex conversation.Execution) error {
		s.followUpsUpdated(ex.Conversation.ID, ex.Run.ID)
		return accepted(ex)
	})
}

// IsFollowUpAttachmentOwner 保留已入库队列及其后续执行引用。
func IsFollowUpAttachmentOwner(item conversation.FollowUp, err error) (bool, error) {
	if errors.Is(err, conversation.ErrNotFound) {
		return false, nil
	}
	return item.Status != conversation.FollowUpRemoved, err
}
