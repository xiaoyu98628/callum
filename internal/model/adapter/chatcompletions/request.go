package chatcompletions

import (
	"errors"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"

	"github.com/xiaoyu98628/callum/internal/model"
)

func buildRequest(req model.Request) (openai.ChatCompletionNewParams, error) {
	if strings.TrimSpace(req.Model) == "" {
		return openai.ChatCompletionNewParams{}, errors.New("模型 ID 不能为空")
	}
	if len(req.Messages) == 0 {
		return openai.ChatCompletionNewParams{}, errors.New("消息不能为空")
	}

	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages))

	for i, message := range req.Messages {
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

	return openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(req.Model),
		Messages: messages,
	}, nil
}
