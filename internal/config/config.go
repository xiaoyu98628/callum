package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
)

type Config struct {
	DataDir  string
	LogLevel slog.Level
}

// Load 只解析应用级配置，项目工作目录由后续的工作空间管理。
func Load(args []string, output io.Writer) (Config, error) {
	if output == nil {
		return Config{}, errors.New("配置帮助输出不能为空")
	}

	flags := flag.NewFlagSet("callum", flag.ContinueOnError)
	flags.SetOutput(output)
	dataDir := flags.String("data-dir", "", "应用数据目录，默认使用系统用户配置目录下的 callum")
	logLevel := flags.String("log-level", "info", "日志级别：debug、info、warn、error")
	if err := flags.Parse(args); err != nil {
		return Config{}, err
	}
	if flags.NArg() != 0 {
		return Config{}, errors.New("当前不接受位置参数")
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(*logLevel)); err != nil {
		return Config{}, fmt.Errorf("日志级别无效: %w", err)
	}
	if level != slog.LevelDebug && level != slog.LevelInfo && level != slog.LevelWarn && level != slog.LevelError {
		return Config{}, errors.New("日志级别必须是 debug、info、warn 或 error")
	}
	if *dataDir == "" {
		var err error
		*dataDir, err = DefaultDataDir()
		if err != nil {
			return Config{}, err
		}
	}
	absDir, err := filepath.Abs(*dataDir)
	if err != nil {
		return Config{}, fmt.Errorf("解析应用数据目录失败: %w", err)
	}
	return Config{DataDir: filepath.Clean(absDir), LogLevel: level}, nil
}
