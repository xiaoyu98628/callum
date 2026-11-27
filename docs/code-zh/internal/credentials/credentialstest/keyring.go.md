```go
// 包 凭据测试 为临时数据库测试提供确定性、无外部状态的主密钥。
// 产品装配不得使用此实现。
package 凭据测试

import (
	上下文包 "context"
	SHA256包 "crypto/sha256"
	凭据包 "github.com/xiaoyu98628/callum/internal/credentials"
)

type 系统凭据库 struct{}

func (系统凭据库) 加载(执行上下文 上下文包.上下文, 名称 string) ([]byte, error) {
	if 错误 := 执行上下文.获取错误(); 错误 != nil {
		return nil, 错误
	}
	键 := SHA256包.校验和256([]byte("callum/isolated-test/" + 名称))
	return 键[:], nil
}
func (系统凭据库) 保存(上下文包.上下文, string, []byte) error { return 凭据包.无效错误 }
```
