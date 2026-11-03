package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/conversation/history"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/tool"
)

// MaxModelRounds 和 MaxToolsPerRun 限制整个循环，辅助标题不计入回答轮次。
const MaxModelRounds = 8
const MaxToolsPerRun = 32

var ErrIterationLimit = errors.New("Agent 调用次数达到上限")

func (e *Engine) loop(ctx context.Context, ex conversation.Execution, resolved agent.Resolved, startTitle func()) error {
	transcripts, err := e.repository.Transcript(ctx, ex.Conversation.ID)
	if err != nil {
		return operationStorageError(ctx, err)
	}
	messages, err := history.Context(ex.History, transcripts)
	if err != nil {
		return err
	}
	var definitions []model.ToolDefinition
	if resolved.Tools && ex.Conversation.WorkspaceID != nil {
		for _, d := range e.options.Tools.Definitions() {
			definitions = append(definitions, d.ToolDefinition)
		}
	}
	denied := map[string]bool{}
	count, totalText := 0, 0
	step := ex.Step
	for round := range MaxModelRounds {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := history.Check(messages); err != nil {
			return err
		}
		if round > 0 {
			id, err := uuid.NewV7()
			if err != nil {
				return err
			}
			now := time.Now().UnixMilli()
			input, err := json.Marshal(map[string]any{"purpose": "answer", "protocol": resolved.Protocol, "round": round + 1, "previous_step_id": step.ID})
			if err != nil {
				return err
			}
			step, err = e.repository.CreateStep(ctx, agent.Step{ID: id.String(), ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, Type: "model_call", Name: "继续生成回答", Status: agent.Pending, Input: input, Model: &resolved.Snapshot, CreatedAt: now, UpdatedAt: now})
			if err != nil {
				return operationStorageError(ctx, err)
			}
			if err := e.event(ex, "step.created", step.ID, "", step); err != nil {
				return err
			}
		}
		callback := func() {}
		if round == 0 {
			callback = startTitle
		}
		result, callErr := e.callModel(ctx, ex, resolved, step, messages, definitions, callback, &totalText)
		if err := e.saveAnswer(ctx, ex, result.Content, result.Text); err != nil {
			return err
		}
		if callErr != nil {
			return callErr
		}
		if len(result.ToolCalls) == 0 {
			return nil
		}
		if count+len(result.ToolCalls) > MaxToolsPerRun {
			return ErrIterationLimit
		}
		count += len(result.ToolCalls)
		// 工具调用轮的文本保留在模型步骤中；回答占位恢复为空，再流式展示下一轮正文。
		if err := e.saveAnswer(ctx, ex, []model.Block{}, ""); err != nil {
			return err
		}
		messages = append(messages, model.Message{Role: model.RoleAssistant, Content: result.Text, ToolCalls: result.ToolCalls})
		for _, call := range result.ToolCalls {
			value, err := e.callTool(ctx, ex, step, call, denied)
			if err != nil {
				return err
			}
			messages = append(messages, value)
		}
	}
	return ErrIterationLimit
}

func (e *Engine) saveAnswer(ctx context.Context, ex conversation.Execution, blocks []model.Block, text string) error {
	if len(blocks) == 0 && text != "" {
		blocks = []model.Block{{Type: "text", Text: text}}
	}
	if blocks == nil {
		blocks = []model.Block{}
	}
	cleanup, cancel := cleanupContext(ctx)
	defer cancel()
	if err := e.repository.SaveMessage(cleanup, ex.Assistant.ID, blocks, agent.Running); err != nil {
		return errors.Join(agent.ErrStorage, err)
	}
	value := ex.Assistant
	value.Content = blocks
	value.UpdatedAt = time.Now().UnixMilli()
	return e.event(ex, "message.updated", "", value.ID, value)
}

func toolFailure(err error) json.RawMessage {
	kind, message := "tool_failed", "工具执行失败"
	switch {
	case errors.Is(err, tool.ErrDenied):
		kind, message = "permission_denied", "操作超出权限范围或已被拒绝"
	case errors.Is(err, tool.ErrInvalid):
		kind, message = "invalid_arguments", "工具参数无效"
	case errors.Is(err, tool.ErrUnavailable):
		kind, message = "unavailable", "工具或文件不可用"
	case errors.Is(err, tool.ErrLimit):
		kind, message = "output_limit", "工具输入或结果超过限制"
	case errors.Is(err, context.Canceled):
		kind, message = "cancelled", "操作已取消"
	case errors.Is(err, context.DeadlineExceeded):
		kind, message = "timeout", "工具执行超时"
	}
	value, _ := json.Marshal(map[string]any{"error": map[string]string{"kind": kind, "message": message}})
	return value
}

// withContextMessage 为跨供应商上下文恢复保存统一消息，不解析供应商协议。
func withContextMessage(output json.RawMessage, message model.Message) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if len(output) > 0 {
		if err := json.Unmarshal(output, &fields); err != nil {
			return nil, err
		}
	}
	if fields == nil {
		fields = map[string]json.RawMessage{}
	}
	raw, err := json.Marshal(message)
	if err != nil {
		return nil, err
	}
	fields["context_message"] = raw
	return json.Marshal(fields)
}
