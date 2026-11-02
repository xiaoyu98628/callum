package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Resolve 将显式或默认数据目录转换为绝对路径，不创建目录或文件。
func Resolve(dataDir string) (string, error) {
	if dataDir == "" {
		var err error
		dataDir, err = defaultDataDir()
		if err != nil {
			return "", err
		}
	}
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return "", fmt.Errorf("计算数据目录: %w", err)
	}
	return abs, nil
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
