package chatcompletions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"

	"github.com/xiaoyu98628/callum/internal/model"
)

func buildRequest(ctx context.Context, req model.Request) (openai.ChatCompletionNewParams, error) {
	if strings.TrimSpace(req.Model) == "" {
		return openai.ChatCompletionNewParams{}, errors.New("模型 ID 不能为空")
	}
	if len(req.Messages) == 0 {
		return openai.ChatCompletionNewParams{}, errors.New("消息不能为空")
	}

	if err := validateToolHistory(req.Messages); err != nil {
		return openai.ChatCompletionNewParams{}, err
	}
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages))
	inputBytes := 0

	for i, message := range req.Messages {
		if message.Reasoning != nil {
			inputBytes += len(*message.Reasoning)
		}
		if len(message.Blocks) == 0 {
			inputBytes += len(message.Content)
		}
		if err := ctx.Err(); err != nil {
			return openai.ChatCompletionNewParams{}, err
		}
		if len(message.Blocks) > 0 {
			var text strings.Builder
			var parts []openai.ChatCompletionContentPartUnionParam
			images := false
			for _, block := range message.Blocks {
				switch block.Type {
				case "text":
					inputBytes += len(block.Text)
					if inputBytes > MaxInputBytes {
						return openai.ChatCompletionNewParams{}, &model.InputLimitError{Dimension: "请求字节", Actual: int64(inputBytes), Limit: MaxInputBytes}
					}
					text.WriteString(block.Text)
					parts = append(parts, openai.TextContentPart(block.Text))
				case "image":
					if !req.AllowImageInput || message.Role != model.RoleUser {
						return openai.ChatCompletionNewParams{}, model.ErrUnsupportedOutput
					}
					data, err := imageData(ctx, block.Path)
					if err != nil {
						return openai.ChatCompletionNewParams{}, err
					}
					inputBytes += len(data)
					if inputBytes > MaxInputBytes {
						return openai.ChatCompletionNewParams{}, &model.InputLimitError{Dimension: "请求字节", Actual: int64(inputBytes), Limit: MaxInputBytes}
					}
					images = true
					parts = append(parts, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{URL: data}))
				default:
					return openai.ChatCompletionNewParams{}, model.ErrUnsupportedOutput
				}
			}
			if images {
				messages = append(messages, openai.UserMessage(parts))
				continue
			}
			message.Content = text.String()
		}
		if inputBytes > MaxInputBytes {
			return openai.ChatCompletionNewParams{}, &model.InputLimitError{Dimension: "请求字节", Actual: int64(inputBytes), Limit: MaxInputBytes}
		}
		switch message.Role {
		case model.RoleSystem:
			messages = append(messages, openai.SystemMessage(message.Content))
		case model.RoleUser:
			messages = append(messages, openai.UserMessage(message.Content))
		case model.RoleAssistant:
			assistant := openai.ChatCompletionAssistantMessageParam{}
			if message.Reasoning != nil {
				assistant.SetExtraFields(map[string]any{"reasoning_content": *message.Reasoning})
			}
			if message.Content != "" || len(message.ToolCalls) == 0 {
				assistant.Content.OfString = openai.String(message.Content)
			}
			for _, call := range message.ToolCalls {
				assistant.ToolCalls = append(assistant.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{ID: call.ID, Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{Name: call.Name, Arguments: string(call.Arguments)}}})
			}
			messages = append(messages, openai.ChatCompletionMessageParamUnion{OfAssistant: &assistant})
		case model.RoleTool:
			if message.ToolCallID == "" {
				return openai.ChatCompletionNewParams{}, model.ErrInvalidResponse
			}
			messages = append(messages, openai.ToolMessage(message.Content, message.ToolCallID))
		default:
			return openai.ChatCompletionNewParams{}, fmt.Errorf("第 %d 条消息的角色暂不支持", i)
		}
	}

	params := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(req.Model),
		Messages: messages,
	}
	if err := addTools(&params, req.Tools); err != nil {
		return openai.ChatCompletionNewParams{}, err
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		return openai.ChatCompletionNewParams{}, err
	}
	// 为 SDK 注入的 stream 和流式选项预留固定头部空间，最终请求仍受 12 MiB 边界约束。
	if len(encoded) > MaxInputBytes-1024 {
		return openai.ChatCompletionNewParams{}, &model.InputLimitError{Dimension: "请求字节", Actual: int64(len(encoded)), Limit: MaxInputBytes - 1024}
	}
	return params, nil
}

// ValidateInput 复用实际请求编码，包含工具定义、推理回传及图片编码的容量检查。
func (c *Client) ValidateInput(ctx context.Context, req model.Request) error {
	_, err := buildRequest(ctx, req)
	return err
}
