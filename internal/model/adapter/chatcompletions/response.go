package chatcompletions

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"

	"github.com/openai/openai-go/v3/option"

	"github.com/xiaoyu98628/callum/internal/model"
)

// Generate 返回文本响应；错误时结果可能包含尚未完整生成的文本。
func (c *Client) Generate(ctx context.Context, req model.Request) (result model.Response, retErr error) {
	params, err := buildRequest(req)
	if err != nil {
		return model.Response{}, err
	}

	var httpResponse *http.Response
	var body *responseBody

	defer func() {
		if body != nil {
			if err := body.Close(); err != nil {
				retErr = errors.Join(retErr, wrapError("close response", httpResponse, err))
			}
		}
	}()

	completion, err := c.sdk.New(
		ctx,
		params,
		option.WithResponseInto(&httpResponse),
		option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
			response, err := next(req)
			// 请求失败时响应体由 HTTP 客户端处理，只接管成功取得的响应。
			if err == nil && response != nil && response.Body != nil {
				body = &responseBody{ReadCloser: response.Body}
				response.Body = body
			}
			return response, err
		}),
	)
	if err != nil {
		return model.Response{}, wrapError("generate", httpResponse, err)
	}

	if completion == nil || len(completion.Choices) != 1 {
		return model.Response{}, wrapError(
			"decode response",
			httpResponse,
			model.ErrInvalidResponse,
		)
	}

	choice := completion.Choices[0]
	if err := validateChoice(choice.RawJSON()); err != nil {
		return model.Response{}, wrapError("decode response", httpResponse, err)
	}
	if err := validateTextOutput(choice.Message.RawJSON()); err != nil {
		return model.Response{}, wrapError("decode response", httpResponse, err)
	}

	result = model.Response{
		Text:      choice.Message.Content,
		RequestID: requestID(httpResponse),
	}

	if len(choice.Message.ToolCalls) > 0 {
		return result, wrapError(
			"decode response",
			httpResponse,
			model.ErrUnsupportedOutput,
		)
	}

	if choice.Message.Refusal != "" {
		return result, wrapError(
			"decode response",
			httpResponse,
			model.ErrRefused,
		)
	}

	if err := checkFinishReason(choice.FinishReason); err != nil {
		// 返回已经收到的文本，同时明确标记调用未成功完成。
		return result, wrapError("finish response", httpResponse, err)
	}
	if !choice.Message.JSON.Content.Valid() {
		return result, wrapError("decode response", httpResponse, model.ErrInvalidResponse)
	}

	return result, nil
}

// responseBody 保存 SDK 已执行的关闭结果，供 Generate 检查清理错误。
// SDK 和 Generate 都可能调用 Close，但底层响应体只能关闭一次。
type responseBody struct {
	io.ReadCloser
	closeOnce sync.Once
	closeErr  error
}

func (b *responseBody) Close() error {
	b.closeOnce.Do(func() {
		b.closeErr = b.ReadCloser.Close()
	})
	return b.closeErr
}

// SDK 的宽松解码可能把数字转换成字符串；使用原始 JSON 校验实际消费的字段。
func validateChoice(raw string) error {
	var choice struct {
		Index        *int64  `json:"index"`
		FinishReason *string `json:"finish_reason"`
	}
	if err := json.Unmarshal([]byte(raw), &choice); err != nil || choice.Index == nil || *choice.Index != 0 {
		return model.ErrInvalidResponse
	}
	return nil
}

func validateTextOutput(raw string) error {
	var output *struct {
		Content   *string           `json:"content"`
		Refusal   *string           `json:"refusal"`
		ToolCalls []json.RawMessage `json:"tool_calls"`
	}
	if err := json.Unmarshal([]byte(raw), &output); err != nil || output == nil {
		return model.ErrInvalidResponse
	}
	return nil
}

func checkFinishReason(reason string) error {
	switch reason {
	case "stop":
		return nil
	case "":
		return io.ErrUnexpectedEOF
	case "length":
		return model.ErrIncomplete
	case "content_filter":
		return model.ErrRefused
	case "tool_calls", "function_call":
		// 当前只支持文本，旧结束标识也仅用于拒绝输出。
		return model.ErrUnsupportedOutput
	default:
		return model.ErrInvalidResponse
	}
}
