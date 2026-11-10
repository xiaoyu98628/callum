// Package system 将数据目录的加密主密钥保存在当前用户的系统凭据库。
package system

import "encoding/hex"

const service = "org.callum.credentials.v1"

// Store 不持有长期系统连接；每次操作负责取消与释放本次资源。
type Store struct{}

func New() Store { return Store{} }

func validName(name string) bool {
	if len(name) != 64 {
		return false
	}
	_, err := hex.DecodeString(name)
	return err == nil
}
