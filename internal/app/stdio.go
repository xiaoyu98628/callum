package app

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"github.com/xiaoyu98628/callum/internal/agent/events"
	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/stdio"
	"github.com/xiaoyu98628/callum/internal/stdio/execution"
	"github.com/xiaoyu98628/callum/internal/stdio/transport"
)

// Run 是独立 stdio 进程入口，接管输入输出，并在连接结束后关闭整个运行时。
func Run(ctx context.Context, cfg config.Config, in io.ReadCloser, out io.WriteCloser, logger *slog.Logger) (retErr error) {
	if in == nil || out == nil {
		return errors.New("输入输出不能为空")
	}
	runtime, err := New(ctx, cfg, logger)
	if err != nil {
		return errors.Join(err, in.Close(), out.Close())
	}
	defer func() { retErr = errors.Join(retErr, runtime.Close()) }()
	return runtime.ServeStdio(ctx, in, out)
}

// ServeStdio 接管一个连接及其发起的任务；EOF 或 system.shutdown 只结束该连接。
// 共享运行时和其他调用方的执行仍然有效；独立进程通过 Run 关闭整个运行时。
func (r *Runtime) ServeStdio(ctx context.Context, in io.ReadCloser, out io.WriteCloser) (retErr error) {
	if ctx == nil || in == nil || out == nil {
		return errors.New("通信初始化参数无效")
	}
	if err := r.beginSession(); err != nil {
		return errors.Join(err, in.Close(), out.Close())
	}
	defer r.sessions.Done()
	sessionCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(r.ctx, cancel)
	defer func() { stop(); cancel() }()
	connection, err := transport.New(sessionCtx, in, out)
	if err != nil {
		return errors.Join(err, in.Close(), out.Close())
	}
	defer func() { retErr = errors.Join(retErr, connection.Close()) }()
	subscription, err := r.Subscribe(events.Filter{})
	if err != nil {
		return err
	}
	defer subscription.Close()
	owner := newSessionRuns(connection.Context(), r.manager)
	defer owner.Close()
	server, err := stdio.NewWithConversations(r.providers, r.calls, r.workspaces, r.conversations, r.engine, r.approvals, r.settings, owner.Register, r.logger)
	if err != nil {
		return err
	}
	if err := server.Ready(connection); err != nil {
		return err
	}
	forwarded := make(chan struct{})
	go func() {
		defer close(forwarded)
		for {
			event, err := subscription.Next(connection.Context())
			if err != nil {
				if !errors.Is(err, io.EOF) && connection.Context().Err() == nil {
					connection.Fail(err)
				}
				return
			}
			if err := execution.EmitPersistent(connection.Notify, event); err != nil {
				connection.Fail(err)
				return
			}
		}
	}()
	err = server.ServeRequests(connection)
	// 正常 EOF 保留事件转发直到本连接的任务清理完毕，再排空订阅和协议队列。
	owner.Close()
	subscription.Close()
	<-forwarded
	return err
}
