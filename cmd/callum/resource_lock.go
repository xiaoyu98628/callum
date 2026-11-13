package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/stdio/protocol"
	"github.com/xiaoyu98628/callum/internal/stdio/transport"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

// 资源安装由客户端执行；此模式仅持有与服务相同的实例锁，不打开数据库。
// 标准流关闭或进程终止即释放锁，避免客户端异常退出留下过期锁状态。
func runResourceLock(args []string, logger *slog.Logger) int {
	flags := flag.NewFlagSet("resource-lock", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	data := flags.String("data-dir", "", "待安装资源的应用数据目录")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			flags.SetOutput(os.Stderr)
			flags.PrintDefaults()
			return 0
		}
		logger.Error("资源锁参数无效", "module", "main")
		return 1
	}
	if *data == "" || flags.NArg() != 0 {
		logger.Error("资源锁必须指定数据目录", "module", "main")
		return 1
	}
	cfg, err := config.Load(*data)
	if err != nil {
		logger.Error("读取资源锁配置失败", "module", "main")
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	signal.Ignore(syscall.SIGPIPE)
	in, out, err := transport.OpenStandardStreams()
	if err == nil {
		err = holdResourceLock(ctx, cfg.DatabasePath, in, out)
	}
	if err != nil {
		logger.Error("资源安装锁失败", "module", "main", "error", err)
		return 1
	}
	return 0
}

func holdResourceLock(ctx context.Context, database string, in io.ReadCloser, out io.WriteCloser) (retErr error) {
	// 输入输出在进入函数时转移所有权，即使实例锁获取失败也必须关闭。
	c, err := transport.New(ctx, in, out)
	if err != nil {
		return errors.Join(err, in.Close(), out.Close())
	}
	defer func() { retErr = errors.Join(retErr, c.Close()) }()
	owner, err := sqlite.AcquireOwnership(database)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, owner.Close()) }()
	if err := c.Notify("system.ready", map[string]any{"service": "callum", "protocol_version": 1, "max_request_bytes": transport.MaxRequestBytes, "max_response_bytes": transport.MaxResponseBytes, "resource_lock": true}); err != nil {
		return err
	}
	scanner := bufio.NewScanner(c.Input())
	scanner.Buffer(make([]byte, 4096), transport.MaxRequestBytes+2)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) > transport.MaxRequestBytes {
			return errors.New("资源锁请求超过边界")
		}
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		req, invalid := protocol.ParseRequest(line)
		if invalid != nil {
			return errors.New("资源锁请求无效")
		}
		var params struct{}
		if req.Method == "system.shutdown" && protocol.DecodeParams(req.Params, &params) == nil {
			return c.Reply(req, map[string]bool{"shutdown": true}, nil)
		}
		if err := c.Reply(req, nil, protocol.MethodNotFound()); err != nil {
			return err
		}
	}
	return c.NormalizeError(scanner.Err())
}
