package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/runtimeassets"
	"github.com/xiaoyu98628/callum/internal/stdio/protocol"
	"github.com/xiaoyu98628/callum/internal/stdio/transport"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

var runtimeStageName = regexp.MustCompile(`^\.runtimes-install-[A-Za-z0-9]+$`)
var runtimeManifestSHA256 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// 资源安装由客户端执行；此模式持有与服务相同的实例锁并校验安装目录，不打开数据库。
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
	dataDir, err := filepath.EvalSymlinks(filepath.Dir(database))
	if err != nil {
		return err
	}
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
		if req.Method == "runtime.validate" {
			if err := validateRuntimeDirectory(c, req, dataDir); err != nil {
				return err
			}
			continue
		}
		if err := c.Reply(req, nil, protocol.MethodNotFound()); err != nil {
			return err
		}
	}
	return c.NormalizeError(scanner.Err())
}

func validateRuntimeDirectory(c *transport.Connection, req protocol.Request, dataDir string) error {
	var params struct {
		Name           string          `json:"name"`
		ManifestSHA256 json.RawMessage `json:"manifest_sha256"`
	}
	if err := protocol.DecodeParams(req.Params, &params); err != nil {
		return c.Reply(req, nil, err)
	}
	if params.Name != "runtimes" && !runtimeStageName.MatchString(params.Name) {
		return c.Reply(req, nil, protocol.InvalidParams())
	}
	var digest [sha256.Size]byte
	if params.ManifestSHA256 != nil {
		var expected string
		if err := json.Unmarshal(params.ManifestSHA256, &expected); err != nil || !runtimeManifestSHA256.MatchString(expected) {
			return c.Reply(req, nil, protocol.InvalidParams())
		}
		if _, err := hex.Decode(digest[:], []byte(expected)); err != nil {
			return c.Reply(req, nil, protocol.InvalidParams())
		}
	}
	path := filepath.Join(dataDir, params.Name)
	info, err := os.Lstat(path)
	valid := false
	if err == nil && info.IsDir() {
		ctx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
		if params.ManifestSHA256 == nil {
			_, err = runtimeassets.Load(ctx, path)
		} else {
			_, err = runtimeassets.LoadWithManifestSHA256(ctx, path, digest)
		}
		cancel()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return c.Reply(req, nil, &protocol.Error{Code: -32603, Message: "运行时目录校验未完成"})
		}
		valid = err == nil
	}
	return c.Reply(req, map[string]bool{"valid": valid}, nil)
}
