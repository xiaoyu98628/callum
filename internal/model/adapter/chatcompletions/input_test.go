package chatcompletions

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/model"
)

func TestInputPreflightUsesEncodedRequestWithoutNetwork(t *testing.T) {
	calls := 0
	client := newTestClient(t, func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("不应访问网络") })
	req := testRequest()
	// 有效文本低于 12 MiB，实际 JSON 转义后超过协议边界。
	req.Messages = []model.Message{{Role: model.RoleUser, Content: strings.Repeat("\x01", (MaxInputBytes/6)+100)}}
	err := client.ValidateInput(t.Context(), req)
	limit, ok := errors.AsType[*model.InputLimitError](err)
	if !ok || !errors.Is(err, model.ErrInputLimit) || limit.Dimension != "请求字节" || limit.Actual <= limit.Limit || calls != 0 {
		t.Fatalf("实际编码预检错误: %+v calls=%d", err, calls)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := client.ValidateInput(ctx, testRequest()); !errors.Is(err, context.Canceled) {
		t.Fatalf("预检忽略取消: %v", err)
	}
}
