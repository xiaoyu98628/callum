package app

import (
	"net"
	"net/http"
	"time"
)

// newHTTPClient 创建应用持有的连接池，调用方在模型任务结束后关闭其空闲连接。
func newHTTPClient(maxConnections int) (*http.Client, *http.Transport) {
	transport := &http.Transport{
		Proxy:             http.ProxyFromEnvironment,
		DialContext:       (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2: true, MaxIdleConns: 64, MaxIdleConnsPerHost: maxConnections,
		MaxConnsPerHost: maxConnections, IdleConnTimeout: 90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	client := &http.Client{Transport: boundedTransport{transport}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return client, transport
}
