package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/model"
)

func (e *Engine) callModel(ctx context.Context, ex conversation.Execution, resolved agent.Resolved, step agent.Step, messages []model.Message, tools []model.ToolDefinition, onAcquired func(), totalText *int) (result model.Response, retErr error) {
	if err := modelMetadata(&step, messages, tools, resolved, ex.Run.Stream); err != nil {
		return result, err
	}
	now := time.Now().UnixMilli()
	step.Status, step.StartedAt, step.UpdatedAt = agent.Running, &now, now
	if err := e.repository.SaveStep(ctx, step); err != nil {
		return result, operationStorageError(ctx, err)
	}
	if err := e.event(ex, "step.updated", step.ID, "", step); err != nil {
		return result, err
	}
	var text strings.Builder
	defer func() {
		if ex.Run.Stream {
			// 捕获结果可能包含导致回调拒绝的分片；消息正文只采用通过累计限额的增量。
			// 原始 Output、usage 和工具调用仍保留在结果中，供步骤审计及成功后的循环使用。
			result.Text = text.String()
			result.Content = nil
		}
		if len(result.Text) > inference.MaxTextBytes {
			result.Text = text.String()
			result.Content = nil
			retErr = errors.Join(retErr, agent.ErrOutputLimit)
		}
		if len(result.Content) == 0 && result.Text != "" {
			result.Content = []model.Block{{Type: "text", Text: result.Text}}
		}
		fillFailure(&result, resolved.Protocol, retErr)
		if retErr == nil {
			var err error
			result.Output, err = withContextMessage(result.Output, model.Message{Role: model.RoleAssistant, Content: result.Text, ToolCalls: result.ToolCalls})
			if err != nil {
				retErr = errors.Join(retErr, err)
			}
		}
		status := agent.Completed
		if retErr != nil {
			var kind, message string
			status, kind, message = failure(retErr)
			step.ErrorKind, step.ErrorMessage = &kind, &message
		}
		now := time.Now().UnixMilli()
		step.Status, step.Output, step.Usage, step.UpdatedAt, step.FinishedAt = status, result.Output, result.Usage, now, &now
		if result.RequestID != "" {
			step.ProviderRequestID = new(result.RequestID)
		}
		cleanup, cancel := cleanupContext(ctx)
		defer cancel()
		if err := e.repository.SaveStep(cleanup, step); err != nil {
			retErr = errors.Join(retErr, agent.ErrStorage, err)
		} else {
			_ = e.event(ex, "step.updated", step.ID, "", step)
		}
	}()
	callCtx, cancel := context.WithTimeout(ctx, e.options.CallTimeout)
	defer cancel()
	request := model.Request{Model: resolved.Snapshot.RemoteModelID, CaptureResult: true, AllowImageInput: resolved.ImageInput, Messages: messages, Tools: tools}
	persisted := 0
	lastPersist := time.Now()
	retErr = e.manager.Call(callCtx, func() error {
		onAcquired()
		if ex.Run.Stream {
			return resolved.Client.Stream(callCtx, request, func(event model.Event) error {
				if event.Result != nil {
					result = *event.Result
					return nil
				}
				if event.RequestID != "" {
					result.RequestID = event.RequestID
				}
				if err := callCtx.Err(); err != nil {
					return err
				}
				if event.Type != model.EventTextDelta {
					return nil
				}
				if *totalText+len(event.Text) > inference.MaxTextBytes {
					return agent.ErrOutputLimit
				}
				*totalText += len(event.Text)
				text.WriteString(event.Text)
				if persisted == 0 || text.Len()-persisted >= 4096 || time.Since(lastPersist) >= 100*time.Millisecond {
					if err := e.repository.SaveMessage(callCtx, ex.Assistant.ID, []model.Block{{Type: "text", Text: text.String()}}, agent.Running); err != nil {
						return errors.Join(agent.ErrStorage, err)
					}
					persisted, lastPersist = text.Len(), time.Now()
				}
				return e.event(ex, "message.delta", step.ID, ex.Assistant.ID, map[string]any{"block_index": 0, "delta": map[string]any{"type": "text", "text": event.Text}})
			})
		}
		var err error
		result, err = resolved.Client.Generate(callCtx, request)
		*totalText += len(result.Text)
		if *totalText > inference.MaxTextBytes {
			result.Text = ""
			result.Content = []model.Block{}
			return errors.Join(err, agent.ErrOutputLimit)
		}
		return err
	})
	if callCtx.Err() != nil {
		retErr = errors.Join(callCtx.Err(), retErr)
	}
	return
}

// modelMetadata 保存实际请求参数、工具定义与上下文指纹，不重复保存整套历史或凭据。
func modelMetadata(step *agent.Step, messages []model.Message, tools []model.ToolDefinition, resolved agent.Resolved, stream bool) error {
	value, err := json.Marshal(messages)
	if err != nil {
		return err
	}
	fingerprint := sha256.Sum256(value)
	var input map[string]any
	if err := json.Unmarshal(step.Input, &input); err != nil {
		return err
	}
	if input == nil {
		input = map[string]any{}
	}
	input["parameters"] = map[string]any{"model": resolved.Snapshot.RemoteModelID, "stream": stream, "image_input": resolved.ImageInput}
	input["context_fingerprint"], input["context_message_count"] = hex.EncodeToString(fingerprint[:]), len(messages)
	step.Input, err = json.Marshal(input)
	if err != nil {
		return err
	}
	if len(tools) > 0 {
		step.Definition, err = json.Marshal(map[string]any{"tools": tools})
	}
	return err
}
