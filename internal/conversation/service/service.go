package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/executor"
	agentservice "github.com/xiaoyu98628/callum/internal/agent/service"
	"github.com/xiaoyu98628/callum/internal/attachment"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/model"
)

// Service 负责会话业务与发送受理；异步执行由注入的 Engine 完成。
type Service struct {
	repository  conversation.Repository
	resolve     agent.Resolver
	manager     *executor.Manager
	engine      *agentservice.Engine
	attachments *attachment.Store
}

func New(repository conversation.Repository, resolve agent.Resolver, manager *executor.Manager, engine *agentservice.Engine, attachments *attachment.Store) (*Service, error) {
	if repository == nil || resolve == nil || manager == nil || engine == nil || attachments == nil {
		return nil, conversation.ErrInvalid
	}
	return &Service{repository: repository, resolve: resolve, manager: manager, engine: engine, attachments: attachments}, nil
}

func validID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed.Version() == 7 && parsed.String() == id
}

func newID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

func (s *Service) Get(ctx context.Context, id string) (conversation.Conversation, error) {
	if !validID(id) {
		return conversation.Conversation{}, conversation.ErrInvalid
	}
	return s.repository.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, f conversation.Filter) ([]conversation.Conversation, error) {
	if f.Limit == 0 {
		f.Limit = 100
	}
	if f.Limit < 1 || f.Limit > 200 || f.Offset < 0 || (f.WorkspaceID != nil && !validID(*f.WorkspaceID)) || (f.Unbound && f.WorkspaceID != nil) {
		return nil, conversation.ErrInvalid
	}
	return s.repository.List(ctx, f)
}

// History 按执行边界返回有界历史，独立压缩即使没有对应消息也可继续翻页。
func (s *Service) History(ctx context.Context, id, before string, limit int) (conversation.HistoryPage, error) {
	if limit == 0 {
		limit = 25
	}
	if !validID(id) || (before != "" && !validID(before)) || limit < 1 || limit > 25 {
		return conversation.HistoryPage{}, conversation.ErrInvalid
	}
	return s.repository.History(ctx, id, before, limit)
}

func (s *Service) Update(ctx context.Context, id string, in conversation.Update) (conversation.Conversation, error) {
	if !validID(id) || (in.WorkspaceID != nil && !validID(*in.WorkspaceID)) || (in.DetachWorkspace && in.WorkspaceID != nil) {
		return conversation.Conversation{}, conversation.ErrInvalid
	}
	change := conversation.Change{Title: in.Title, WorkspaceID: in.WorkspaceID, DetachWorkspace: in.DetachWorkspace, Pinned: in.Pinned, Archived: in.Archived, Deleted: in.Deleted}
	if change.Title != nil {
		value := strings.TrimSpace(*change.Title)
		if value == "" || len(value) > 1024 || !utf8.ValidString(value) {
			return conversation.Conversation{}, conversation.ErrInvalid
		}
		change.Title = &value
	}
	if in.ModelID != "" {
		if !validID(in.ModelID) {
			return conversation.Conversation{}, conversation.ErrInvalid
		}
		resolved, err := s.resolve(ctx, in.ModelID)
		if err != nil {
			return conversation.Conversation{}, err
		}
		change.Model = &resolved.Snapshot
	}
	value, err := s.repository.Update(ctx, id, change)
	if err == nil {
		s.engine.ConversationUpdated(value)
	}
	return value, err
}

func validate(in conversation.Send) error {
	if in.ConversationID != "" && !validID(in.ConversationID) {
		return conversation.ErrInvalid
	}
	if in.ModelID != "" && !validID(in.ModelID) {
		return conversation.ErrInvalid
	}
	if in.WorkspaceID != nil && (!validID(*in.WorkspaceID) || in.ConversationID != "") {
		return conversation.ErrInvalid
	}
	if in.MessageID != "" && (!validID(in.MessageID) || in.ConversationID == "") {
		return conversation.ErrInvalid
	}
	if in.ExpectedRunID != "" && (in.MessageID == "" || !validID(in.ExpectedRunID)) {
		return conversation.ErrInvalid
	}
	if len(in.RetainedAttachments) > 0 && (in.MessageID == "" || in.ExpectedRunID == "") {
		return conversation.ErrInvalid
	}
	if len(in.Content)+len(in.RetainedAttachments) > 64 {
		return conversation.ErrInvalid
	}
	meaningful := len(in.RetainedAttachments) != 0
	size := 0
	for _, block := range in.Content {
		size += len(block.Text) + len(block.Path) + len(block.AttachmentRef) + len(block.Name) + len(block.MIME)
		switch block.Type {
		case "text":
			if block.Path != "" || block.AttachmentRef != "" || block.Name != "" || block.MIME != "" || block.Size != 0 || !utf8.ValidString(block.Text) {
				return conversation.ErrInvalid
			}
			meaningful = meaningful || strings.TrimSpace(block.Text) != ""
		case "image", "file":
			if block.Text != "" || (block.AttachmentRef != "" && (block.Path != "" || in.MessageID == "")) || (block.AttachmentRef == "" && (!filepath.IsAbs(block.Path) || filepath.Clean(block.Path) != block.Path)) {
				return conversation.ErrInvalid
			}
			if !attachment.ValidName(block.Name) || len(block.MIME) > 255 || block.Size < 0 {
				return conversation.ErrInvalid
			}
			meaningful = true
		default:
			return conversation.ErrInvalid
		}
	}
	if !meaningful || size > 1<<20 {
		return conversation.ErrInvalid
	}
	return nil
}

// Send 在事务提交后调用 accepted；回调成功后才启动异步调用和发布执行事件。
// ctx 只用于受理；执行使用独立运行上下文，输入内容在受理前复制。
func (s *Service) Send(ctx context.Context, in conversation.Send, accepted func(conversation.Execution) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if accepted == nil {
		return conversation.ErrInvalid
	}
	if err := validate(in); err != nil {
		return err
	}
	in.Content = append([]model.Block(nil), in.Content...)
	create := in.ConversationID == ""
	if create {
		if in.ModelID == "" {
			return conversation.ErrInvalid
		}
		id, err := newID()
		if err != nil {
			return err
		}
		in.ConversationID = id
	} else if in.ModelID == "" {
		conv, err := s.repository.Get(ctx, in.ConversationID)
		if err != nil {
			return err
		}
		in.ModelID = conv.Model.ID
	}
	ids := make([]string, 4)
	for i := range ids {
		id, err := newID()
		if err != nil {
			return err
		}
		ids[i] = id
	}
	lease, err := s.manager.ReserveAgentContext(ctx, ids[0], in.ConversationID)
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
	if err := lookup.Err(); err != nil {
		return err
	}
	if resolved.Client == nil || resolved.Snapshot.ID != in.ModelID || strings.TrimSpace(resolved.Snapshot.RemoteModelID) == "" {
		return conversation.ErrInvalid
	}
	if len(in.RetainedAttachments) > 0 {
		in.Content, err = s.retainedAttachments(lookup, in)
		if err != nil {
			return err
		}
	}
	hasAttachments := false
	for _, block := range in.Content {
		if block.Type == "image" {
			if !resolved.ImageInput {
				return inference.ErrUnsupported
			}
		}
		if block.Type == "image" || block.Type == "file" {
			hasAttachments = true
		}
	}
	var batch *attachment.Batch
	if hasAttachments {
		batch, err = s.attachments.Stage(lookup, in.ConversationID, ids[0], in.Content)
		if err != nil {
			return err
		}
		if validator, ok := resolved.Client.(model.InputValidator); ok {
			req := model.Request{Model: resolved.Snapshot.RemoteModelID, AllowImageInput: resolved.ImageInput,
				Messages: []model.Message{{Role: model.RoleUser, Blocks: in.Content}}}
			if err := validator.ValidateInput(lookup, req); err != nil {
				return errors.Join(err, batch.Discard())
			}
		}
	}
	stream := true
	if in.Stream != nil {
		stream = *in.Stream
	}
	ex, err := s.repository.Begin(lookup, conversation.Begin{Create: create, ConversationID: in.ConversationID, WorkspaceID: in.WorkspaceID, RunID: ids[0], UserMessageID: ids[1], AssistantMessageID: ids[2], StepID: ids[3], ResendMessageID: in.MessageID, ResendRunID: in.ExpectedRunID, Model: resolved.Snapshot, Protocol: resolved.Protocol, Content: in.Content, Stream: stream})
	batch.Finish(lookup, err == nil)
	if err != nil {
		return err
	}
	acceptedValue := ex
	if ex.Run.SettingsSnapshot != nil {
		// 受理回调借用独立快照，不能通过指针修改即将执行的权限设置。
		snapshot := ex.Run.SettingsSnapshot.Clone()
		acceptedValue.Run.SettingsSnapshot = &snapshot
	}
	if ex.Context != nil {
		value := ex.Context.Clone()
		acceptedValue.Context = &value
	}
	if err := accepted(acceptedValue); err != nil {
		s.engine.Abandon(lease.Context(), ex, context.Canceled)
		return err
	}
	lease.Start(func(ctx context.Context) { s.engine.Execute(ctx, lease, ex, resolved) })
	return nil
}
