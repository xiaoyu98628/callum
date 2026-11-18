// Package artifact 提供通用的本地读图和交付文件保留工具，不解析 Office 格式。
package artifact

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"

	"github.com/xiaoyu98628/callum/internal/attachment"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/imageinput"
	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/internal/fileaccess"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/internal/arguments"
)

type fileTool struct {
	access     *attachment.Access
	protection tool.ProtectedData
	view       bool
}

func Tools(access *attachment.Access, protection tool.ProtectedData) []tool.Tool {
	return []tool.Tool{fileTool{access, protection, true}, fileTool{access, protection, false}}
}

func (t fileTool) Definition() tool.Definition {
	name, description := "publish_artifact", "读取本地文件并保存独立会话交付副本，在回答中显示可另存的文件卡片。本轮临时文件必须通过此工具保存后才能交付。源文件不改写；path 支持绝对路径，相对路径需要工作区。"
	if t.view {
		name, description = "view_image", "读取本地图片并作为视觉输入提供给模型，用于检查渲染页面、截图或图表。需要用户所选模型启用 image 输入。path 支持绝对路径，相对路径需要工作区。图片保存为本次工具的稳定上下文，原文件不修改。"
	}
	return tool.Definition{Name: name, Description: description, Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`), Source: "builtin", Version: "1", Effects: []tool.Effect{tool.EffectRead}}
}

type args struct {
	Path string `json:"path"`
}

func (fileTool) Validate(raw json.RawMessage) (json.RawMessage, error) {
	var a args
	if _, err := arguments.Decode(raw, &a); err != nil {
		return nil, err
	}
	if strings.TrimSpace(a.Path) == "" || strings.ContainsRune(a.Path, 0) {
		return nil, tool.ErrInvalid
	}
	return json.Marshal(a)
}
func (t fileTool) Prepare(ctx context.Context, scope tool.Scope, raw json.RawMessage) (tool.Invocation, error) {
	return t.PrepareWithPermissions(ctx, scope, raw, tool.ExecutionPermissions{})
}
func (t fileTool) PrepareWithPermissions(ctx context.Context, scope tool.Scope, raw json.RawMessage, permissions tool.ExecutionPermissions) (tool.Invocation, error) {
	normalized, err := t.Validate(raw)
	if err != nil {
		return nil, err
	}
	var a args
	if err := json.Unmarshal(normalized, &a); err != nil {
		return nil, err
	}
	if t.access == nil || permissions.RunID == "" || (t.view && !permissions.AllowImageInput) {
		return nil, tool.ErrUnavailable
	}
	handle, err := fileaccess.Prepare(ctx, scope, a.Path, fileaccess.File, t.protection)
	if err != nil {
		return nil, err
	}
	return &invocation{Handle: handle, access: t.access, permissions: permissions, view: t.view}, nil
}

type invocation struct {
	*fileaccess.Handle
	access      *attachment.Access
	permissions tool.ExecutionPermissions
	view        bool
}

func (p *invocation) Execute(ctx context.Context) (tool.Result, error) {
	result := tool.Result{Outcome: tool.OutcomeNone}
	if err := p.Check(ctx); err != nil {
		return result, err
	}
	if p.Info().Size() > imageinput.MaxSourceBytes {
		return result, tool.ErrLimit
	}
	var data []byte
	buffer := make([]byte, 64<<10)
	for {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		n, err := p.File().Read(buffer)
		data = append(data, buffer[:n]...)
		if len(data) > imageinput.MaxSourceBytes {
			return result, tool.ErrLimit
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return result, err
		}
	}
	kind := "file"
	if p.view {
		kind = "image"
	}
	block, err := p.access.SaveGenerated(ctx, p.permissions.ConversationID, p.permissions.RunID, kind, filepath.Base(p.Target().Path), data, !p.view)
	if err != nil {
		return result, err
	}
	result.Output, err = json.Marshal(map[string]any{"attachment": block, "published": !p.view})
	if p.view {
		result.Images = []model.Block{block}
	}
	return result, err
}
