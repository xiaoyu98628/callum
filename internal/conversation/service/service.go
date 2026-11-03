package service

import (
	"context"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/run"
	"github.com/xiaoyu98628/callum/internal/run/executor"
	runservice "github.com/xiaoyu98628/callum/internal/run/service"
)

// Service 负责会话业务与发送受理；异步执行由注入的 Engine 完成。
type Service struct {
	repository conversation.Repository
	resolve    run.Resolver
	manager    *executor.Manager
	engine     *runservice.Engine
}

func New(repository conversation.Repository, resolve run.Resolver, manager *executor.Manager, engine *runservice.Engine) (*Service, error) {
	if repository == nil || resolve == nil || manager == nil || engine == nil {
		return nil, conversation.ErrInvalid
	}
	return &Service{repository: repository, resolve: resolve, manager: manager, engine: engine}, nil
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

func (s *Service) Messages(ctx context.Context, id string, before int64, limit int) ([]conversation.Message, error) {
	if limit == 0 {
		limit = 100
	}
	if !validID(id) || before < 0 || limit < 1 || limit > 200 {
		return nil, conversation.ErrInvalid
	}
	return s.repository.Messages(ctx, id, before, limit)
}

func (s *Service) Update(ctx context.Context, id string, change conversation.Change, modelID string) (conversation.Conversation, error) {
	if !validID(id) || (change.WorkspaceID != nil && !validID(*change.WorkspaceID)) || (change.DetachWorkspace && change.WorkspaceID != nil) {
		return conversation.Conversation{}, conversation.ErrInvalid
	}
	if change.Title != nil {
		value := strings.TrimSpace(*change.Title)
		if value == "" || len(value) > 1024 || !utf8.ValidString(value) {
			return conversation.Conversation{}, conversation.ErrInvalid
		}
		change.Title = &value
	}
	if modelID != "" {
		if !validID(modelID) {
			return conversation.Conversation{}, conversation.ErrInvalid
		}
		resolved, err := s.resolve(ctx, modelID)
		if err != nil {
			return conversation.Conversation{}, err
		}
		change.Model = &resolved.Snapshot
	}
	return s.repository.Update(ctx, id, change)
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
	if len(in.Content) == 0 || len(in.Content) > 64 {
		return conversation.ErrInvalid
	}
	meaningful := false
	size := 0
	for _, block := range in.Content {
		size += len(block.Text) + len(block.Path)
		switch block.Type {
		case "text":
			if block.Path != "" || !utf8.ValidString(block.Text) {
				return conversation.ErrInvalid
			}
			meaningful = meaningful || strings.TrimSpace(block.Text) != ""
		case "image":
			if block.Text != "" || !filepath.IsAbs(block.Path) || filepath.Clean(block.Path) != block.Path {
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

// Send 在事务接受成功后回复，回复成功排队后启动异步调用。
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
	lease, err := s.manager.ReserveContext(ctx, ids[0], in.ConversationID)
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
	for _, block := range in.Content {
		if block.Type == "image" && !resolved.ImageInput {
			return inference.ErrUnsupported
		}
	}
	stream := true
	if in.Stream != nil {
		stream = *in.Stream
	}
	ex, err := s.repository.Begin(lookup, conversation.Begin{Create: create, ConversationID: in.ConversationID, WorkspaceID: in.WorkspaceID, RunID: ids[0], UserMessageID: ids[1], AssistantMessageID: ids[2], StepID: ids[3], ResendMessageID: in.MessageID, Model: resolved.Snapshot, Protocol: resolved.Protocol, Content: in.Content, Stream: stream})
	if err != nil {
		return err
	}
	if err := accepted(ex); err != nil {
		s.engine.Abandon(lease.Context(), ex, context.Canceled)
		return err
	}
	lease.Start(func(ctx context.Context) { s.engine.Execute(ctx, lease, ex, resolved) })
	return nil
}
