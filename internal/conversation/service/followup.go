package service

import (
	"context"
	"errors"
	"strings"

	"github.com/xiaoyu98628/callum/internal/attachment"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/conversation/history"
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
		run, err := s.engine.GetRun(ctx, in.ExpectedRunID)
		if err != nil || run.ConversationID != in.ConversationID {
			return out, conversation.ErrFollowUpChanged
		}
		in.ModelID = run.Model.ID
	}
	resolved, err := s.resolve(ctx, in.ModelID)
	if err != nil {
		return out, err
	}
	id, err := newID()
	if err != nil {
		return out, err
	}
	var batch *attachment.Batch
	hasAttachments := false
	for _, block := range content {
		if block.Type == "image" && !resolved.ImageInput {
			return out, inference.ErrUnsupported
		}
		hasAttachments = hasAttachments || block.Type == "image" || block.Type == "file"
	}
	if hasAttachments {
		batch, err = s.attachments.Stage(ctx, in.ConversationID, id, content)
		if err != nil {
			return out, err
		}
		defer func() { batch.Finish(ctx, err == nil) }()
		if validator, ok := resolved.Client.(model.InputValidator); ok {
			request := model.Request{Model: resolved.Snapshot.RemoteModelID, AllowImageInput: resolved.ImageInput,
				Messages: []model.Message{{Role: model.RoleUser, Blocks: history.ModelContent(content)}}}
			if err := validator.ValidateInput(ctx, request); err != nil {
				return out, err
			}
		}
	}
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
		err = s.engine.AcceptSteering(in.ExpectedRunID, write)
	} else {
		_, err = write()
		if err == nil {
			s.engine.FollowUpsUpdated(in.ConversationID, in.ExpectedRunID)
		}
	}
	return out, err
}

func (s *Service) ChangeFollowUp(ctx context.Context, in conversation.FollowUpChange) (out conversation.FollowUp, err error) {
	if !validID(in.ConversationID) || !validID(in.ID) || in.Revision < 1 ||
		(in.Action == "steer") != (in.ExpectedRunID != "") || (in.ExpectedRunID != "" && !validID(in.ExpectedRunID)) ||
		(in.Action == "edit") != (in.Text != nil) {
		return out, conversation.ErrInvalid
	}
	item, err := s.repository.FollowUp(ctx, in.ConversationID, in.ID)
	if err != nil {
		return out, err
	}
	var content []model.Block
	if in.Action == "edit" {
		for _, block := range item.Content {
			if block.Type != "text" {
				content = append(content, block)
			}
		}
		if t := strings.TrimSpace(*in.Text); t != "" {
			content = append([]model.Block{{Type: "text", Text: t}}, content...)
		}
		// 已保管的附件仅由同一条队列记录保留，客户端不能提交引用。
		if err := validate(conversation.Send{ConversationID: in.ConversationID, MessageID: item.ID, Content: content}); err != nil {
			return out, err
		}
	}
	if in.Action == "steer" {
		run, err := s.engine.GetRun(ctx, in.ExpectedRunID)
		if err != nil || run.ConversationID != in.ConversationID {
			return out, conversation.ErrFollowUpChanged
		}
		resolved, err := s.resolve(ctx, run.Model.ID)
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
	if in.Action == "steer" {
		err = s.engine.AcceptSteering(in.ExpectedRunID, write)
	} else {
		_, err = write()
		if err == nil {
			s.engine.FollowUpsUpdated(in.ConversationID, in.ExpectedRunID)
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

func (s *Service) StartFollowUp(ctx context.Context, in conversation.FollowUpStart, accepted func(conversation.Execution) error) error {
	if !validID(in.ConversationID) || !validID(in.ID) || in.Revision < 1 || (in.AfterRunID != "" && !validID(in.AfterRunID)) {
		return conversation.ErrInvalid
	}
	item, err := s.repository.FollowUp(ctx, in.ConversationID, in.ID)
	if err != nil {
		return err
	}
	if item.Status != conversation.FollowUpQueued || item.Revision != in.Revision {
		return conversation.ErrFollowUpChanged
	}
	return s.send(ctx, conversation.Send{ConversationID: in.ConversationID, ModelID: item.ModelID, Content: item.Content}, &in, func(ex conversation.Execution) error {
		s.engine.FollowUpsUpdated(ex.Conversation.ID, ex.Run.ID)
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
