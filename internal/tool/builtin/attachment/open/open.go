// Package open 将当前会话的受管附件交付为一次执行的独立只读输入副本。
package open

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/xiaoyu98628/callum/internal/attachment"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/internal/arguments"
)

type openTool struct{ access *attachment.Access }

func New(access *attachment.Access) tool.Tool { return openTool{access: access} }

func (openTool) Definition() tool.Definition {
	return tool.Definition{Name: "open_attachment", Description: "取得当前会话普通文件附件的只读输入副本。attachment_ref 必须来自当前会话的附件元数据。返回 path、work_directory 和文件元数据；使用 run_command 的内置 python3/node 与相应技能脚本按需读取。副本在本轮执行结束后清理，后续追问需重新调用本工具。此操作不把文件全文自动发送给模型，也不开放应用数据目录。", Parameters: json.RawMessage(`{"type":"object","properties":{"attachment_ref":{"type":"string"}},"required":["attachment_ref"],"additionalProperties":false}`), Source: "builtin", Version: "1", Effects: []tool.Effect{tool.EffectRead}}
}

type input struct {
	Reference string `json:"attachment_ref"`
}

func (openTool) Validate(raw json.RawMessage) (json.RawMessage, error) {
	var value input
	if _, err := arguments.Decode(raw, &value); err != nil {
		return nil, err
	}
	if value.Reference == "" || len(value.Reference) > 1024 {
		return nil, tool.ErrInvalid
	}
	return json.Marshal(value)
}

func (t openTool) Prepare(ctx context.Context, scope tool.Scope, raw json.RawMessage) (tool.Invocation, error) {
	return t.PrepareWithPermissions(ctx, scope, raw, tool.ExecutionPermissions{})
}

func (t openTool) PrepareWithPermissions(ctx context.Context, _ tool.Scope, raw json.RawMessage, permissions tool.ExecutionPermissions) (tool.Invocation, error) {
	normalized, err := t.Validate(raw)
	if err != nil {
		return nil, err
	}
	var value input
	if err := json.Unmarshal(normalized, &value); err != nil {
		return nil, err
	}
	block, file, err := t.access.Metadata(ctx, permissions.ConversationID, permissions.RunID, value.Reference)
	if err != nil {
		return nil, errors.Join(tool.ErrDenied, err)
	}
	return &invocation{access: t.access, conversationID: permissions.ConversationID, runID: permissions.RunID, block: block, file: file}, nil
}

type invocation struct {
	access                *attachment.Access
	conversationID, runID string
	block                 model.Block
	file                  *attachment.File
	once                  sync.Once
	err                   error
}

func (*invocation) Target() *tool.Target { return nil }

func (p *invocation) Execute(ctx context.Context) (tool.Result, error) {
	path, work, err := p.access.Export(ctx, p.conversationID, p.runID, p.block, p.file)
	if err != nil {
		return tool.Result{Outcome: tool.OutcomeNone}, errors.Join(tool.ErrUnavailable, err)
	}
	output, err := json.Marshal(map[string]any{"path": path, "work_directory": work, "name": p.block.Name, "mime": p.block.MIME, "size": p.block.Size, "attachment_ref": p.block.AttachmentRef, "lifetime": "current_run"})
	return tool.Result{Output: output, Outcome: tool.OutcomeNone}, err
}

func (p *invocation) Close() error { p.once.Do(func() { p.err = p.file.Close() }); return p.err }
