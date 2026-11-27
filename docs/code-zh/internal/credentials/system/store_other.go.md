```go
//go:build !darwin && !windows && !linux

package 系统

import (
	上下文包 "context"
	凭据包 "github.com/xiaoyu98628/callum/internal/credentials"
)

func (存储) 加载(上下文包.上下文, string) ([]byte, error) { return nil, 凭据包.不可用错误 }
func (存储) 保存(上下文包.上下文, string, []byte) error   { return 凭据包.不可用错误 }
```
