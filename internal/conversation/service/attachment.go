package service

import (
	"context"
	"path/filepath"
	"slices"

	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/imageinput"
)

// ReadAttachmentChunk 只读当前分支中消息明确持有的副本，不接受客户端指定任意路径。
// 每次核对完整哈希，再返回不超过 1 MiB 的片段，保持 stdio 响应有界。
func (s *Service) ReadAttachmentChunk(ctx context.Context, conversationID, messageID string, index int, offset int64) (conversation.AttachmentChunk, error) {
	var result conversation.AttachmentChunk
	if !validID(conversationID) || !validID(messageID) || index < 0 || index >= 64 || offset < 0 {
		return result, conversation.ErrInvalid
	}
	message, err := s.repository.Message(ctx, conversationID, messageID)
	if err != nil {
		return result, err
	}
	if index >= len(message.Content) {
		return result, conversation.ErrNotFound
	}
	block := message.Content[index]
	if (block.Type != "file" && block.Type != "image") || block.AttachmentRef == "" {
		return result, conversation.ErrNotFound
	}
	data, mime, err := s.attachments.Read(ctx, block.AttachmentRef)
	if err != nil {
		return result, err
	}
	if offset > int64(len(data)) || (offset == int64(len(data)) && offset != 0) {
		return result, conversation.ErrInvalid
	}
	end := min(offset+(1<<20), int64(len(data)))
	chunk := make([]byte, end-offset)
	copy(chunk, data[offset:end])
	name := block.Name
	if name == "" {
		name = filepath.Base(filepath.FromSlash(block.AttachmentRef))
	}
	result = conversation.AttachmentChunk{Name: name, MIME: mime, Size: int64(len(data)), Offset: offset, Data: chunk}
	if end < int64(len(data)) {
		result.NextOffset = new(end)
	}
	return result, nil
}

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
