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
	"github.com/xiaoyu98628/callum/internal/model/tokens"
)

func (e *Engine) callModel(ctx context.Context, ex conversation.Execution, resolved agent.Resolved, step agent.Step, messages []model.Message, tools []model.ToolDefinition, onAcquired func(), totalText *int) (result model.Response, retErr error) {
	if ex.Context != nil {
		var input map[string]any
		if err := json.Unmarshal(step.Input, &input); err != nil {
			return result, err
		}
		input["context_version_id"] = ex.Context.ID
		var err error
		step.Input, err = json.Marshal(input)
		if err != nil {
			return result, err
		}
	}
	if err := modelMetadata(&step, messages, tools, resolved, ex.Run.Stream); err != nil {
		return result, err
	}
	if err := tokenMetadata(&step, ex, resolved, messages, tools); err != nil {
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
	var text, reasoning strings.Builder
	usageStream := e.contextStream(ex)
	reasoningSeen := false
	defer func() {
		if ex.Run.Stream {
			// 捕获结果可能包含导致回调拒绝的分片；消息正文只采用通过累计限额的增量。
			// 原始 Output、usage 和工具调用仍保留在结果中，供步骤审计及成功后的循环使用。
			result.Text = text.String()
			result.Reasoning = nil
			if reasoningSeen {
				result.Reasoning = new(reasoning.String())
			}
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
		if result.Reasoning != nil {
			var err error
			result.Output, err = withReasoning(result.Output, *result.Reasoning)
			retErr = errors.Join(retErr, err)
		}
		if retErr == nil {
			var err error
			result.Output, err = withContextMessage(result.Output, model.Message{Role: model.RoleAssistant, Content: result.Text, Reasoning: result.Reasoning, ToolCalls: result.ToolCalls})
			if err != nil {
				retErr = errors.Join(retErr, err)
			}
		}
		var usageErr error
		result.Output, usageErr = withActualInputTokens(result.Output, result.InputTokens)
		retErr = errors.Join(retErr, usageErr)
		e.observeInputTokens(ex, result.InputTokens)
		finalMessages := append([]model.Message(nil), messages...)
		if result.Text != "" || result.Reasoning != nil || len(result.ToolCalls) > 0 {
			finalMessages = append(finalMessages, model.Message{Role: model.RoleAssistant, Content: result.Text, Reasoning: result.Reasoning, ToolCalls: result.ToolCalls})
		}
		var finalMeter tokens.Meter
		finalMeter.Observe(messages, tools, result.InputTokens)
		e.recordContext(ex, resolved, finalMessages, tools, &finalMeter, true)
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
				if event.Type == model.EventReasoningDelta {
					if *totalText+len(event.Text) > inference.MaxTextBytes {
						return agent.ErrOutputLimit
					}
					*totalText += len(event.Text)
					reasoningSeen = true
					reasoning.WriteString(event.Text)
					usageStream.add(event.Text, true)
					// 等待正文前也定期保存推理，进程中断后仍可查询已接受的片段。
					if reasoning.Len() == len(event.Text) || time.Since(lastPersist) >= 100*time.Millisecond {
						var err error
						step.Output, err = withReasoning(nil, reasoning.String())
						if err != nil {
							return err
						}
						if err := e.repository.SaveStep(callCtx, step); err != nil {
							return operationStorageError(callCtx, err)
						}
						lastPersist = time.Now()
					}
					return e.event(ex, "reasoning.delta", step.ID, "", map[string]any{"text": event.Text})
				}
				if event.Type != model.EventTextDelta {
					return nil
				}
				if *totalText+len(event.Text) > inference.MaxTextBytes {
					return agent.ErrOutputLimit
				}
				*totalText += len(event.Text)
				text.WriteString(event.Text)
				usageStream.add(event.Text, false)
				if persisted == 0 || text.Len()-persisted >= 4096 || time.Since(lastPersist) >= 100*time.Millisecond {
					if err := e.repository.SaveMessage(callCtx, ex.Assistant.ID, []model.Block{{Type: "text", Text: text.String()}}, agent.Running); err != nil {
						// 取消可能恰好打断正文写入；只将同一上下文错误视为取消/超时。
						// 真实写入错误仍保留存储失败标记，不能因同时取消而被掩盖。
						if ctxErr := callCtx.Err(); ctxErr != nil && errors.Is(err, ctxErr) {
							return err
						}
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
		if result.Reasoning != nil {
			*totalText += len(*result.Reasoning)
		}
		if *totalText > inference.MaxTextBytes {
			result.Text = ""
			result.Reasoning = nil
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

// withActualInputTokens 持久化统一的实际输入投影；缺失时保留 null，不估算补齐 usage。
func withActualInputTokens(output json.RawMessage, input *int64) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if len(output) > 0 {
		if err := json.Unmarshal(output, &fields); err != nil {
			return nil, err
		}
	}
	if fields == nil {
		fields = map[string]json.RawMessage{}
	}
	if input != nil && (*input < 0 || *input > 1_000_000_000_000) {
		input = nil
	}
	value, err := json.Marshal(map[string]*int64{"actual_input_tokens": input})
	if err != nil {
		return nil, err
	}
	fields["context_usage"] = value
	return json.Marshal(fields)
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

// tokenMetadata 与实际 usage 分开记录；预算取自本次受理的模型配置及设置快照。
func tokenMetadata(step *agent.Step, ex conversation.Execution, resolved agent.Resolved, messages []model.Message, tools []model.ToolDefinition) error {
	if ex.Run.SettingsSnapshot == nil || ex.Run.SettingsSnapshot.Compaction == nil {
		return nil
	}
	cfg := ex.Run.SettingsSnapshot.Compaction
	var input map[string]any
	if err := json.Unmarshal(step.Input, &input); err != nil {
		return err
	}
	budget, err := cfg.InputBudget(resolved.ContextWindow, resolved.MaxOutputTokens)
	if err != nil {
		if cfg.Enabled {
			return model.ErrTokenBudget
		}
		return nil
	}
	input["token_budget"] = map[string]any{"input_limit": budget, "context_window": resolved.ContextWindow, "max_output_tokens": resolved.MaxOutputTokens, "estimated_input_tokens": tokens.Estimate(messages, tools, cfg.ImageTokenEstimate), "estimation_method": tokens.Method}
	step.Input, err = json.Marshal(input)
	return err
}

// withReasoning 保存统一展示投影；失败或中断时也保留已接受的推理，不冒充完整上下文。
func withReasoning(output json.RawMessage, text string) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if len(output) > 0 {
		if err := json.Unmarshal(output, &fields); err != nil {
			return nil, err
		}
	}
	if fields == nil {
		fields = map[string]json.RawMessage{}
	}
	value, err := json.Marshal(text)
	if err != nil {
		return nil, err
	}
	fields["reasoning"] = value
	return json.Marshal(fields)
}
