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

	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages))
	inputBytes := 0

	for i, message := range req.Messages {
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
						return openai.ChatCompletionNewParams{}, model.ErrInputLimit
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
						return openai.ChatCompletionNewParams{}, model.ErrInputLimit
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
			return openai.ChatCompletionNewParams{}, model.ErrInputLimit
		}
		switch message.Role {
		case model.RoleSystem:
			messages = append(messages, openai.SystemMessage(message.Content))
		case model.RoleUser:
			messages = append(messages, openai.UserMessage(message.Content))
		case model.RoleAssistant:
			messages = append(messages, openai.AssistantMessage(message.Content))
		default:
			return openai.ChatCompletionNewParams{}, fmt.Errorf("第 %d 条消息的角色暂不支持", i)
		}
	}

	params := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(req.Model),
		Messages: messages,
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		return openai.ChatCompletionNewParams{}, err
	}
	// 为 SDK 注入的 stream 和流式选项预留固定头部空间，最终请求仍受 12 MiB 边界约束。
	if len(encoded) > MaxInputBytes-1024 {
		return openai.ChatCompletionNewParams{}, model.ErrInputLimit
	}
	return params, nil
}
