package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPClientUsesRequestDeadlineForResponseHeaders(t *testing.T) {
	const responseDelay = 100 * time.Millisecond
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timer := time.NewTimer(responseDelay)
		defer timer.Stop()
		select {
		case <-r.Context().Done():
		case <-timer.C:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(server.Close)
	client, transport := newHTTPClient(1)
	t.Cleanup(transport.CloseIdleConnections)
	if transport.ResponseHeaderTimeout != 0 {
		t.Fatalf("响应头等待仍有独立期限: %v", transport.ResponseHeaderTimeout)
	}
	for _, tc := range []struct {
		name    string
		timeout time.Duration
	}{
		{"可在请求期限内等待慢响应", 2 * time.Second},
		{"超过请求期限时结束等待", 20 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), tc.timeout)
			defer cancel()
			request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := client.Do(request)
			if response != nil {
				defer response.Body.Close()
			}
			if tc.timeout < responseDelay {
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("预期请求超时，收到 %v", err)
				}
				return
			}
			if err != nil || response.StatusCode != http.StatusNoContent {
				t.Fatalf("请求期限内未取得响应: %v, %v", response, err)
			}
		})
	}
}
