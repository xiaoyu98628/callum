package chatcompletions

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/xiaoyu98628/callum/internal/model"
)

// Config 是 Chat Completions 客户端的运行时连接配置。
type Config struct {
	// BaseURL 是 API 根地址，例如 https://gateway.example/v1。
	BaseURL string

	// APIKey 是运行时读取的凭据。
	APIKey string
}

// Client 将 Chat Completions 协议转换为统一的模型接口。
type Client struct {
	sdk openai.ChatCompletionService
}

var _ model.Client = (*Client)(nil)

// New 创建模型客户端，HTTP 连接池的生命周期由调用方管理。
// 连接配置只使用显式参数，不继承 SDK 的环境变量配置。
func New(cfg Config, httpClient *http.Client) (*Client, error) {
	if httpClient == nil {
		return nil, errors.New("http client 不能为空")
	}

	u, err := url.Parse(cfg.BaseURL)
	if err != nil ||
		(u.Scheme != "http" && u.Scheme != "https") ||
		u.Host == "" ||
		u.User != nil ||
		u.RawQuery != "" ||
		u.Fragment != "" {
		return nil, errors.New("API 根地址无效")
	}

	sdk := openai.NewChatCompletionService(
		option.WithBaseURL(strings.TrimRight(u.String(), "/")+"/"),
		option.WithAPIKey(cfg.APIKey),
		option.WithHTTPClient(httpClient),

		// 重试策略由上层明确决定。
		option.WithMaxRetries(0),
	)

	return &Client{sdk: sdk}, nil
}
