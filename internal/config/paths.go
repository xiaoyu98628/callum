package config

import (
	"fmt"
	"os"
	"path/filepath"
)

func DefaultDataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("获取应用数据目录失败: %w", err)
	}
	return filepath.Join(base, "callum"), nil
}

func (c Config) DatabasePath() string {
	return filepath.Join(c.DataDir, "callum.db")
}

func (c Config) LogPath() string {
	return filepath.Join(c.DataDir, "logs", "callum.log")
}
