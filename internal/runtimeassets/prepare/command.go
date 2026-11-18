package prepare

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// RunCommand 执行独立的资源准备命令；offlineOnly 调用方不能通过参数启用下载。
// 此入口不创建应用数据目录、数据库或 stdio 服务。
func RunCommand(ctx context.Context, args []string, logger *slog.Logger, offlineOnly bool) error {
	flags := flag.NewFlagSet("prepare-runtimes", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	source := flags.String("sources", "", "固定版本来源清单")
	output := flags.String("output", "", "客户端 runtimes 资源根目录")
	cache := flags.String("cache", "", "本地归档目录")
	target := flags.String("target", "", "可选的平台目录名；省略时准备全部目标")
	download := flags.Bool("download", false, "维护资源时允许下载缺失归档；默认严格离线")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			flags.SetOutput(os.Stderr)
			flags.Usage()
		}
		return err
	}
	if *source == "" || *output == "" || *cache == "" || flags.NArg() != 0 {
		return fmt.Errorf("必须指定 --sources、--output 和 --cache")
	}
	targets, err := ReadSources(*source)
	if err != nil {
		return err
	}
	outputPath, err := filepath.Abs(*output)
	if err != nil {
		return err
	}
	cachePath, err := filepath.Abs(*cache)
	if err != nil {
		return err
	}
	if *download && offlineOnly {
		return errors.New("此资源准备入口仅支持离线归档")
	}
	matched := false
	for _, entry := range targets {
		if *target != "" && entry.Directory != *target {
			continue
		}
		matched = true
		if err := Install(ctx, entry, outputPath, cachePath, !*download); err != nil {
			return fmt.Errorf("准备 %s: %w", entry.Directory, err)
		}
		logger.Info("运行时资源已校验", "platform", entry.Directory)
	}
	if !matched {
		return fmt.Errorf("来源清单没有指定目标")
	}
	return nil
}
