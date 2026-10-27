package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultDataDir 按构建模式返回默认的数据目录。
func DefaultDataDir() (string, error) {
	var base string
	var err error
	if releaseBuild {
		base, err = os.UserHomeDir()
	} else {
		base, err = os.Getwd()
	}
	if err != nil {
		return "", fmt.Errorf("获取应用数据目录失败: %w", err)
	}
	return filepath.Join(base, ".callum"), nil
}

func (c Config) DatabasePath() string {
	return filepath.Join(c.DataDir, "callum.db")
}

func (c Config) LogPath() string {
	return filepath.Join(c.DataDir, "logs", "callum.log")
}
