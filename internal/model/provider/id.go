package provider

import (
	"github.com/google/uuid"
)

// ValidID 检查标识是否为小写、带连字符的标准 UUIDv7，避免不同文本表示指向同一标识。
func ValidID(id string) bool {
	u, err := uuid.Parse(id)
	return err == nil && u.Version() == 7 && u.Variant() == uuid.RFC4122 && u.String() == id
}
