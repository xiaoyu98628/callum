package chatcompletions

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
)

func TestWrapErrorRetainsCauseWithoutExposingDetails(t *testing.T) {
	cause := errors.New("测试原始错误")
	secret := "test-secret-do-not-log"
	err := wrapError("generate", &http.Response{
		StatusCode: http.StatusBadGateway,
		Header:     http.Header{"X-Request-Id": {testRequestID}},
	}, fmt.Errorf("credential=%s: %w", secret, cause))
	if !errors.Is(err, cause) {
		t.Fatal("原始错误链丢失")
	}
	callErr := assertCallError(t, err, http.StatusBadGateway, testRequestID)
	if callErr.Operation != "generate" {
		t.Errorf("调用阶段为 %q", callErr.Operation)
	}
	if strings.Contains(err.Error(), secret) {
		t.Error("普通错误输出包含原始敏感信息")
	}
}

func TestWrapErrorFallsBackToSDKResponse(t *testing.T) {
	apiErr := &openai.Error{
		StatusCode: http.StatusTooManyRequests,
		Response: &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     http.Header{"X-Request-Id": {testRequestID}},
		},
	}
	err := wrapError("generate", nil, fmt.Errorf("SDK 调用失败: %w", apiErr))
	assertCallError(t, err, http.StatusTooManyRequests, testRequestID)
	if unwrapped, ok := errors.AsType[*openai.Error](err); !ok || unwrapped != apiErr {
		t.Fatal("供应商错误链丢失")
	}
}
