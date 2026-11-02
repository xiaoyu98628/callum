package config

import (
	"path/filepath"
	"time"

	"github.com/xiaoyu98628/callum/internal/config/paths"
)

// Config 保存应用的数据路径和模型执行限制，不创建目录或打开文件。
type Config struct {
	DataDir           string
	DatabasePath      string
	MaxConcurrentRuns int
	CallTimeout       time.Duration
}

// Load 优先使用显式目录；未指定时根据构建标签选择默认目录。
// 开发构建从工作目录向上定位 Callum 项目；release 构建使用用户家目录。
// 路径解析失败时返回错误，不切换到其他模式的数据目录。
func Load(dataDir string) (Config, error) {
	abs, err := paths.Resolve(dataDir)
	if err != nil {
		return Config{}, err
	}
	return Config{DataDir: abs, DatabasePath: filepath.Join(abs, "callum.db"),
		MaxConcurrentRuns: DefaultMaxConcurrentRuns, CallTimeout: DefaultCallTimeout}, nil
}
