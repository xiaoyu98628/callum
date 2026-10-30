package stdio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/model/provider"
)

// Options 指定执行限制，协议字节限制与写入等待时限由服务统一管理。
type Options struct {
	MaxConcurrentRuns int
	CallTimeout       time.Duration
}

// Server 处理逐行 JSON-RPC 消息，复用配置存储和统一模型调用接口。
// 创建者持有数据库和 HTTP 连接池；每次 Serve 独立持有并关闭输入输出及模型任务。
type Server struct {
	store        *provider.Store
	resolve      inference.Resolver
	options      Options
	logger       *slog.Logger
	writeTimeout time.Duration
}

// New 创建通信服务，不打开资源；参数错误时返回错误。
func New(store *provider.Store, resolve inference.Resolver, options Options, logger *slog.Logger) (*Server, error) {
	if store == nil || resolve == nil || logger == nil || options.MaxConcurrentRuns < 1 || options.MaxConcurrentRuns > 64 || options.CallTimeout <= 0 {
		return nil, errors.New("通信服务初始化参数无效")
	}
	return &Server{store: store, resolve: resolve, options: options, logger: logger, writeTimeout: defaultWriteTimeout}, nil
}

func (s *Server) info() map[string]any {
	return map[string]any{"service": "callum", "protocol_version": 1, "max_concurrent_runs": s.options.MaxConcurrentRuns,
		"call_timeout_ms": s.options.CallTimeout.Milliseconds(), "max_request_bytes": maxRequestBytes,
		"max_response_bytes": maxResponseBytes, "max_text_bytes": inference.MaxTextBytes}
}

// Serve 持续处理请求直到 EOF、关闭请求、取消或通信失败。
// 本方法取得 in/out 的所有权，两者的 Close 必须能中断正在进行的 Read/Write。
// 正常关闭会取消并等待执行、排空输出后关闭管道；通信失败或外部取消时立即中断管道。
func (s *Server) Serve(ctx context.Context, in io.ReadCloser, out io.WriteCloser) (retErr error) {
	if in == nil || out == nil {
		return errors.New("协议输入输出不能为空")
	}
	c := newConnection(ctx, in, out, s.writeTimeout)
	closed := make(chan struct{})
	stopClose := context.AfterFunc(c.ctx, func() { c.closeStreams(); close(closed) })
	go c.writeLoop()
	manager, err := inference.New(c.ctx, s.resolve, s.options.MaxConcurrentRuns, s.options.CallTimeout, func(event inference.Event) error {
		params := struct {
			RunID             string    `json:"run_id"`
			Text              *string   `json:"text,omitempty"`
			ProviderRequestID string    `json:"provider_request_id,omitempty"`
			Error             *rpcError `json:"error,omitempty"`
		}{RunID: event.RunID, Text: event.Text, ProviderRequestID: event.ProviderRequestID}
		if event.Err != nil {
			params.Error = publicError(event.Err, true)
		}
		return c.notify("run."+event.Type, params)
	})
	defer func() {
		if manager != nil {
			manager.Close()
		}
		close(c.queue)
		<-c.done
		// Stop 返回 false 时必须等关闭回调结束，避免与资源清理并发访问 closeErr。
		if !stopClose() {
			<-closed
		}
		c.closeStreams()
		cause := context.Cause(c.ctx)
		c.cancel(context.Canceled)
		if ctx.Err() != nil {
			// 外部取消会主动关闭管道，随之产生的读写错误不代表新的通信故障。
			if errors.Is(retErr, ctx.Err()) || errors.Is(retErr, io.ErrClosedPipe) || errors.Is(retErr, os.ErrClosed) {
				retErr = nil
			}
			if cause != nil && !errors.Is(cause, context.Cause(ctx)) {
				retErr = errors.Join(retErr, cause)
			}
			retErr = errors.Join(retErr, c.closeErr)
		} else {
			retErr = errors.Join(retErr, cause, c.closeErr)
		}
	}()
	if err != nil {
		return err
	}
	if err := c.notify("system.ready", s.info()); err != nil {
		return err
	}
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 4096), maxRequestBytes+2)
	for scanner.Scan() {
		if c.ctx.Err() != nil {
			return context.Cause(c.ctx)
		}
		line := scanner.Bytes()
		if len(line) > maxRequestBytes {
			return errors.New("协议请求超过 1 MiB 限制")
		}
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		stop, err := s.handle(c, manager, line)
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("读取协议输入失败: %w", err)
	}
	return nil
}

func (s *Server) handle(c *connection, manager *inference.Manager, line []byte) (bool, error) {
	req, parseErr := parseRequest(line)
	if parseErr != nil {
		return false, c.send(response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: parseErr})
	}
	// 配置操作有独立短期超时；不能将它作为异步模型执行的父上下文。
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()
	if req.Method == "model.invoke" {
		return false, s.invoke(ctx, c, manager, req)
	}
	var result any
	var rpcErr *rpcError
	stop := false
	switch req.Method {
	case "system.info", "system.shutdown":
		rpcErr = decodeParams(req.Params, &struct{}{})
		if rpcErr == nil {
			result = s.info()
			stop = req.Method == "system.shutdown"
		}
	case "run.cancel":
		var p struct {
			RunID string `json:"run_id"`
		}
		rpcErr = decodeParams(req.Params, &p)
		if rpcErr == nil && p.RunID == "" {
			rpcErr = invalidParams()
		}
		if rpcErr == nil {
			result = map[string]any{"cancel_requested": manager.Cancel(p.RunID)}
		}
	default:
		result, rpcErr = s.catalog(ctx, req)
	}
	if rpcErr != nil && rpcErr.Code == -32603 {
		s.logger.Error("配置请求处理失败", "module", "stdio", "method", req.Method, "request_id", string(req.ID))
	}
	return stop, c.reply(req, result, rpcErr)
}
