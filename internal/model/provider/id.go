package provider

import (
	"fmt"

	"github.com/google/uuid"
)

// ValidID 检查标识是否为小写、带连字符的标准 UUIDv7，避免不同文本表示指向同一标识。
func ValidID(id string) bool {
	u, err := uuid.Parse(id)
	return err == nil && u.Version() == 7 && u.Variant() == uuid.RFC4122 && u.String() == id
}

func newID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("生成模型配置标识失败: %w", err)
	}
	return id.String(), nil
}
