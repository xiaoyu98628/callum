package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"time"
)

const (
	// DefaultMaxConcurrentRuns 是活动执行和实际模型请求的共同数量上限。
	DefaultMaxConcurrentRuns = 4
	// DefaultCallTimeout 是单次模型请求的超时（不含审批等待）。
	DefaultCallTimeout = 5 * time.Minute
)

type toolFlags struct {
	tool *time.Duration
}

func flags(w io.Writer) (*flag.FlagSet, *string, *int, *time.Duration, toolFlags) {
	f := flag.NewFlagSet("callum", flag.ContinueOnError)
	f.SetOutput(w)
	dir := f.String("data-dir", "", "数据目录；未指定时按开发或发布构建选择")
	limit := f.Int("max-concurrent-runs", DefaultMaxConcurrentRuns, "同时活动执行及实际模型请求的上限（1 至 64）")
	timeout := f.Duration("call-timeout", DefaultCallTimeout, "每次模型调用的超时，例如 5m")
	extra := toolFlags{
		tool: f.Duration("tool-timeout", 30*time.Second, "单次工具执行超时"),
	}
	return f, dir, limit, timeout, extra
}

// LoadArgs 解析启动参数，再计算路径；不会创建数据库或修改文件。
func LoadArgs(args []string) (Config, error) {
	f, dir, limit, timeout, extra := flags(io.Discard)
	if err := f.Parse(args); err != nil {
		return Config{}, err
	}
	if f.NArg() != 0 {
		return Config{}, errors.New("不接受位置参数，请通过 stdin 发送协议请求")
	}
	if *limit < 1 || *limit > 64 || *timeout <= 0 {
		return Config{}, errors.New("并发上限必须为 1 至 64，调用超时必须大于零")
	}
	cfg, err := Load(*dir)
	if err != nil {
		return Config{}, err
	}
	if *extra.tool <= 0 {
		return Config{}, errors.New("工具超时必须大于零")
	}
	cfg.MaxConcurrentRuns, cfg.CallTimeout = *limit, *timeout
	cfg.ToolTimeout = *extra.tool
	return cfg, nil
}

// WriteUsage 向指定输出写入帮助，程序入口应传入 stderr。
func WriteUsage(w io.Writer) {
	f, _, _, _, _ := flags(w)
	fmt.Fprintln(w, "用法：callum [启动参数]，启动后每行输入一条 JSON 请求。")
	f.PrintDefaults()
}
