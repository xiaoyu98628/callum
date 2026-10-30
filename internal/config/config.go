package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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
	if dataDir == "" {
		var err error
		dataDir, err = defaultDataDir()
		if err != nil {
			return Config{}, err
		}
	}
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return Config{}, fmt.Errorf("计算数据目录: %w", err)
	}
	return Config{DataDir: abs, DatabasePath: filepath.Join(abs, "callum.db"),
		MaxConcurrentRuns: DefaultMaxConcurrentRuns, CallTimeout: DefaultCallTimeout}, nil
}

func defaultDataDir() (string, error) {
	if releaseBuild {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("读取用户家目录: %w", err)
		}
		if !filepath.IsAbs(home) {
			return "", errors.New("用户家目录必须是绝对路径")
		}
		return filepath.Join(home, ".callum"), nil
	}
	root, err := findProjectRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".callum", "dev"), nil
}

func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("读取开发工作目录: %w", err)
	}
	for {
		content, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("读取开发项目标识: %w", err)
		}
		if err == nil && isCallumModule(content) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("未找到 Callum 项目根目录，请从项目目录启动或显式指定数据目录")
		}
		dir = parent
	}
}

func isCallumModule(content []byte) bool {
	for line := range strings.Lines(string(content)) {
		line, _, _ = strings.Cut(line, "//")
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[0] != "module" {
			continue
		}
		module := fields[1]
		if strings.HasPrefix(module, "\"") || strings.HasPrefix(module, "`") {
			var err error
			module, err = strconv.Unquote(module)
			if err != nil {
				return false
			}
		}
		// 校验模块标识，避免在其他项目中启动时使用错误的数据目录。
		return module == "github.com/xiaoyu98628/callum"
	}
	return false
}
