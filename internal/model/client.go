package model

import "context"

// Client 是业务调用模型的统一接口。
type Client interface {
	Generate(ctx context.Context, req Request) (Response, error)

	// Stream 在当前 goroutine 中同步调用 emit。
	// emit 应及时返回；返回错误会停止读取并释放响应资源。
	// 方法返回 nil 才表示整个调用成功。
	Stream(
		ctx context.Context,
		req Request,
		emit func(Event) error,
	) error
}
