package service

import (
	"fmt"

	"github.com/google/uuid"
)

func newID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("生成模型配置标识失败: %w", err)
	}
	return id.String(), nil
}
