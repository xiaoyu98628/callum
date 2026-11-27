```go
package 推理调用

import (
	上下文包 "context"

	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

// 最大文本字节 限制单次调用累计的文本字节数，不代表模型的 词元 容量。
const 最大文本字节 = 1 << 20

// 解析器 为一次调用取得配置快照和统一客户端，不得在返回后持有数据库事务。
type 解析器 func(上下文包.上下文, string) (模型包.客户端, string, error)

// 请求 由调用方提供完整文本上下文，不负责保存会话历史。
type 请求 struct {
	模型标识 string
	流    bool
	消息集  []模型包.消息
}

// 事件 是执行事件，错误 仅供通信边界转换，不能直接序列化错误链。
type 事件 struct {
	类型      string
	执行标识    string
	文本      *string
	供应商请求标识 string
	错误      error
}
```
