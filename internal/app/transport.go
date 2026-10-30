package app

import (
	"errors"
	"io"
	"net/http"
)

// 限制 SDK 读取的原始响应，防止非流式响应在应用检查文本长度前无限分配内存。
// 此限制也适用于流式响应的累计传输字节，与文本大小和 Token 容量相互独立。
const maxUpstreamBytes = 16 << 20

var errResponseLimit = errors.New("模型原始响应超过 16 MiB 限制")

type boundedTransport struct{ base http.RoundTripper }

func (t boundedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(req)
	if err == nil && response != nil && response.Body != nil {
		response.Body = &boundedBody{ReadCloser: response.Body, remaining: maxUpstreamBytes}
	}
	return response, err
}

type boundedBody struct {
	io.ReadCloser
	remaining int
}

func (b *boundedBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if b.remaining == 0 {
		var probe [1]byte
		n, err := b.ReadCloser.Read(probe[:])
		if n > 0 {
			return 0, errResponseLimit
		}
		return 0, err
	}
	if len(p) > b.remaining {
		p = p[:b.remaining]
	}
	n, err := b.ReadCloser.Read(p)
	b.remaining -= n
	return n, err
}
