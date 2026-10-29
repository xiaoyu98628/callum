package chatcompletions

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/xiaoyu98628/callum/internal/model"
)

// Stream 同步发送文本增量，响应关闭成功后再发送完成事件。
func (c *Client) Stream(ctx context.Context, req model.Request, emit func(model.Event) error) error {
	if emit == nil {
		return errors.New("流式事件回调不能为空")
	}

	params, err := buildRequest(req)
	if err != nil {
		return err
	}

	httpResponse, err := c.streamText(ctx, params, emit)
	if err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return wrapError("stream", httpResponse, err)
	}

	if err := emit(model.Event{
		Type:      model.EventCompleted,
		RequestID: requestID(httpResponse),
	}); err != nil {
		return wrapError("callback", httpResponse, err)
	}

	return nil
}

// streamText 负责关闭响应，返回的响应仅供上层读取状态和请求标识。
func (c *Client) streamText(
	ctx context.Context,
	params openai.ChatCompletionNewParams,
	emit func(model.Event) error,
) (httpResponse *http.Response, retErr error) {
	stream := c.sdk.NewStreaming(
		ctx,
		params,
		option.WithResponseInto(&httpResponse),
	)

	defer func() {
		if err := stream.Close(); err != nil {
			retErr = errors.Join(
				retErr,
				wrapError("close stream", httpResponse, err),
			)
		}
	}()

	var finishReason string

	for stream.Next() {
		if err := ctx.Err(); err != nil {
			return httpResponse, wrapError("stream", httpResponse, err)
		}

		chunk := stream.Current()
		var choices []json.RawMessage
		if err := json.Unmarshal([]byte(chunk.JSON.Choices.Raw()), &choices); err != nil || choices == nil {
			return httpResponse, wrapError("decode stream", httpResponse, model.ErrInvalidResponse)
		}

		// 用量等附加分片的 choices 可以为空数组。
		if len(choices) == 0 {
			continue
		}

		if len(choices) != 1 || len(chunk.Choices) != 1 {
			return httpResponse, wrapError(
				"decode stream",
				httpResponse,
				model.ErrInvalidResponse,
			)
		}

		choice := chunk.Choices[0]
		if err := validateChoice(choice.RawJSON()); err != nil {
			return httpResponse, wrapError("decode stream", httpResponse, err)
		}
		if err := validateTextOutput(choice.Delta.RawJSON()); err != nil {
			return httpResponse, wrapError("decode stream", httpResponse, err)
		}

		// 已验证的服务会在结束后发送带空 choice 的用量分片。
		// 仅跳过纯用量分片，继续生成内容或再次指定结束原因仍视为异常。
		if finishReason != "" {
			if isTrailingUsageChunk(chunk) {
				continue
			}
			return httpResponse, wrapError(
				"decode stream",
				httpResponse,
				model.ErrInvalidResponse,
			)
		}

		if len(choice.Delta.ToolCalls) > 0 {
			return httpResponse, wrapError(
				"decode stream",
				httpResponse,
				model.ErrUnsupportedOutput,
			)
		}

		if choice.Delta.Refusal != "" {
			return httpResponse, wrapError(
				"decode stream",
				httpResponse,
				model.ErrRefused,
			)
		}

		if choice.Delta.Content != "" {
			if err := emit(model.Event{
				Type:      model.EventTextDelta,
				Text:      choice.Delta.Content,
				RequestID: requestID(httpResponse),
			}); err != nil {
				return httpResponse, wrapError("callback", httpResponse, err)
			}
		}

		if choice.FinishReason != "" {
			finishReason = choice.FinishReason
		}
	}

	if err := stream.Err(); err != nil {
		return httpResponse, wrapError("read stream", httpResponse, err)
	}

	if err := ctx.Err(); err != nil {
		return httpResponse, wrapError("stream", httpResponse, err)
	}

	// 读取结束不等于生成成功，必须检查模型的结束原因。
	if err := checkFinishReason(finishReason); err != nil {
		return httpResponse, wrapError("finish stream", httpResponse, err)
	}

	return httpResponse, nil
}

func isTrailingUsageChunk(chunk openai.ChatCompletionChunk) bool {
	if len(chunk.Choices) != 1 {
		return false
	}
	choice := chunk.Choices[0]
	if reason := choice.JSON.FinishReason.Raw(); reason != "" && reason != "null" {
		return false
	}

	// 检查原始 delta 必须是空对象，避免漏掉当前未支持的生成字段。
	var delta map[string]json.RawMessage
	if err := json.Unmarshal([]byte(choice.Delta.RawJSON()), &delta); err != nil || delta == nil || len(delta) != 0 {
		return false
	}

	var usage map[string]json.RawMessage
	return json.Unmarshal([]byte(chunk.JSON.Usage.Raw()), &usage) == nil && usage != nil
}
