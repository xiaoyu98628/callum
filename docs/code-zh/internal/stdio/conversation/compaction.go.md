```go
package 会话

import (
	上下文包 "context"

	业务域包 "github.com/xiaoyu98628/callum/internal/conversation"
	协议包 "github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

func (当前处理器 *处理器) 压缩(执行上下文 上下文包.上下文, 请求 协议包.请求, 回复 func(any, *协议包.错误) error) error {
	var 输入 业务域包.压缩
	if 错误 := 协议包.解码参数(请求.参数, &输入); 错误 != nil {
		return 回复(nil, 错误)
	}
	已回复 := false
	错误 := 当前处理器.服务.压缩(执行上下文, 输入, func(本次执行 业务域包.执行) error {
		if 当前处理器.已受理 != nil {
			if 错误 := 当前处理器.已受理(本次执行.执行.标识); 错误 != nil {
				return 错误
			}
		}
		已回复 = true
		return 回复(map[string]string{"conversation_id": 本次执行.会话.标识, "run_id": 本次执行.执行.标识}, nil)
	})
	if 错误 != nil {
		if 已回复 {
			return 错误
		}
		return 回复(nil, 公开错误(错误))
	}
	return nil
}
```
