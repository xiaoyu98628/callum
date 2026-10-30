package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/database"
	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/adapter/chatcompletions"
	"github.com/xiaoyu98628/callum/internal/model/provider"
	"github.com/xiaoyu98628/callum/internal/stdio"
)

// Run 持有数据库、HTTP 连接池和输入输出，初始化失败或运行结束时均负责清理。
// cfg 必须由配置模块计算，ctx 必须是服务运行上下文。
func Run(ctx context.Context, cfg config.Config, in io.ReadCloser, out io.WriteCloser, logger *slog.Logger) (retErr error) {
	if in == nil || out == nil {
		return errors.New("输入输出不能为空")
	}
	owned := true
	defer func() {
		if owned {
			retErr = errors.Join(retErr, in.Close(), out.Close())
		}
	}()
	if logger == nil || cfg.MaxConcurrentRuns < 1 || cfg.MaxConcurrentRuns > 64 || cfg.CallTimeout <= 0 {
		return errors.New("应用运行配置无效")
	}
	initCtx, initCancel := context.WithTimeout(ctx, 10*time.Second)
	db, err := database.Open(initCtx, cfg.DatabasePath)
	initCancel()
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, db.Close()) }()
	store, err := provider.NewStore(db.DB)
	if err != nil {
		return err
	}
	// 独立连接池由本次应用运行持有，不修改或关闭进程的默认连接池。
	transport := &http.Transport{
		Proxy:             http.ProxyFromEnvironment,
		DialContext:       (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2: true, MaxIdleConns: 64, MaxIdleConnsPerHost: cfg.MaxConcurrentRuns,
		MaxConnsPerHost: cfg.MaxConcurrentRuns, IdleConnTimeout: 90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: boundedTransport{transport}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resolve := func(ctx context.Context, id string) (model.Client, string, error) {
		runtime, err := store.Resolve(ctx, id)
		if err != nil {
			return nil, "", err
		}
		if runtime.Protocol != "chatcompletions" || len(runtime.Model.ModelType) != 0 {
			return nil, "", inference.ErrUnsupported
		}
		adapter, err := chatcompletions.New(chatcompletions.Config{BaseURL: runtime.BaseURL, APIKey: runtime.APIKey}, client)
		return adapter, runtime.Model.RemoteModelID, err
	}
	server, err := stdio.New(store, resolve, stdio.Options{MaxConcurrentRuns: cfg.MaxConcurrentRuns, CallTimeout: cfg.CallTimeout}, logger)
	if err != nil {
		return err
	}
	owned = false
	return server.Serve(ctx, in, out)
}
