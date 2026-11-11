package service

import (
	"context"
	"path/filepath"
	"slices"

	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/imageinput"
)

// PreviewAttachment 返回有界预览。历史图从固定消息索引读取，草稿使用本地路径。
func (s *Service) PreviewAttachment(ctx context.Context, conversationID, messageID string, index int, path string) ([]byte, string, error) {
	var block model.Block
	if path != "" {
		if conversationID != "" || messageID != "" || index != 0 || !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return nil, "", conversation.ErrInvalid
		}
		block.Path = path
	} else {
		if !validID(conversationID) || !validID(messageID) || index < 0 || index >= 64 {
			return nil, "", conversation.ErrInvalid
		}
		message, err := s.repository.Message(ctx, conversationID, messageID)
		if err != nil {
			return nil, "", err
		}
		if message.Role != string(model.RoleUser) || index >= len(message.Content) || message.Content[index].Type != "image" {
			return nil, "", conversation.ErrNotFound
		}
		block = message.Content[index]
	}
	var data []byte
	var mime string
	var err error
	if block.AttachmentRef != "" {
		data, mime, err = s.attachments.Read(ctx, block.AttachmentRef)
	} else {
		data, mime, err = imageinput.Read(ctx, block.Path)
	}
	if err != nil {
		return nil, "", err
	}
	data, err = imageinput.Prepare(ctx, data, mime)
	return data, mime, err
}

// retainedAttachments 从正在编辑的消息取回引用，客户端只能选择该消息的块索引。
func (s *Service) retainedAttachments(ctx context.Context, in conversation.Send) ([]model.Block, error) {
	message, err := s.repository.Message(ctx, in.ConversationID, in.MessageID)
	if err != nil {
		return nil, err
	}
	if message.Role != string(model.RoleUser) || message.RunID != in.ExpectedRunID {
		return nil, conversation.ErrConflict
	}
	seen := make(map[int]bool, len(in.RetainedAttachments))
	retained := make([]model.Block, 0, len(in.RetainedAttachments))
	for _, index := range in.RetainedAttachments {
		if index < 0 || index >= len(message.Content) || (message.Content[index].Type != "image" && message.Content[index].Type != "file") || seen[index] {
			return nil, conversation.ErrInvalid
		}
		seen[index] = true
		retained = append(retained, message.Content[index])
	}
	firstImage := slices.IndexFunc(in.Content, func(block model.Block) bool { return block.Type == "image" || block.Type == "file" })
	if firstImage < 0 {
		firstImage = len(in.Content)
	}
	in.Content = slices.Insert(in.Content, firstImage, retained...)
	in.RetainedAttachments = nil
	if err := validate(in); err != nil {
		return nil, err
	}
	return in.Content, nil
}
