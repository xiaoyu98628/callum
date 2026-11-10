// Package credentialstest 为临时数据库测试提供确定性、无外部状态的主密钥。
// 产品装配不得使用此实现。
package credentialstest

import (
	"context"
	"crypto/sha256"
	"github.com/xiaoyu98628/callum/internal/credentials"
)

type Keyring struct{}

func (Keyring) Load(ctx context.Context, name string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := sha256.Sum256([]byte("callum/isolated-test/" + name))
	return key[:], nil
}
func (Keyring) Save(context.Context, string, []byte) error { return credentials.ErrInvalid }
