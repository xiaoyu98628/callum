// Package id 统一持久化标识的 UUIDv7 文本校验，不决定业务错误或标识归属。
package id

import "github.com/google/uuid"

// ValidV7 只接受小写、带连字符且使用 RFC4122 variant 的标准 UUIDv7。
func ValidV7(value string) bool {
	u, err := uuid.Parse(value)
	return err == nil && u.Version() == 7 && u.Variant() == uuid.RFC4122 && u.String() == value
}
