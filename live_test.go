package callum

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/adapter/chatcompletions"
)

func TestLiveModel(t *testing.T) {
	// 普通测试默认跳过，避免意外调用真实服务。
	if os.Getenv("CALLUM_LIVE_TEST") != "1" {
		t.Skip("设置 CALLUM_LIVE_TEST=1 后运行真实 API 测试")
	}

	baseURL := os.Getenv("CALLUM_BASE_URL")
	apiKey := os.Getenv("CALLUM_API_KEY")
	modelID := os.Getenv("CALLUM_MODEL")
	if baseURL == "" || apiKey == "" || modelID == "" {
		t.Fatal("请设置 CALLUM_BASE_URL、CALLUM_API_KEY 和 CALLUM_MODEL")
	}

	// 测试创建并管理自己的 HTTP 连接池。
	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()

	client, err := chatcompletions.New(chatcompletions.Config{
		BaseURL: baseURL,
		APIKey:  apiKey,
	}, &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}

	req := model.Request{
		Model: modelID,
		Messages: []model.Message{
			{Role: model.RoleUser, Content: "请用一句中文介绍你自己。"},
		},
	}

	t.Run("Generate", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
		defer cancel()

		response, err := client.Generate(ctx, req)
		if err != nil {
			t.Fatalf("非流式请求失败：%v", err)
		}

		t.Logf("回答：%s", response.Text)
		t.Logf("请求 ID：%s", response.RequestID)
	})

	t.Run("Stream", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
		defer cancel()

		completed := false
		err := client.Stream(ctx, req, func(event model.Event) error {
			switch event.Type {
			case model.EventTextDelta:
				t.Logf("文本增量：%q", event.Text)
			case model.EventCompleted:
				completed = true
				t.Logf("流式完成，请求 ID：%s", event.RequestID)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("流式请求失败：%v", err)
		}
		if !completed {
			t.Fatal("未收到完成事件")
		}
	})
}
