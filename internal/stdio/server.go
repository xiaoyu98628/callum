package stdio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/model/provider"
)

// Server 只负责协议处理，配置业务服务与执行管理器均由装配层注入。
// 连接、任务、数据库及 HTTP 资源的创建与关闭顺序由调用方管理。
type Server struct {
	providers *provider.Service
	runs      *inference.Manager
	logger    *slog.Logger
}

// New 创建协议处理对象，不创建或关闭外部资源。
func New(providers *provider.Service, runs *inference.Manager, logger *slog.Logger) (*Server, error) {
	if providers == nil || runs == nil || logger == nil {
		return nil, errors.New("通信服务初始化参数无效")
	}
	return &Server{providers: providers, runs: runs, logger: logger}, nil
}

func (s *Server) info() map[string]any {
	limit, timeout := s.runs.Limits()
	return map[string]any{"service": "callum", "protocol_version": 1, "max_concurrent_runs": limit,
		"call_timeout_ms": timeout.Milliseconds(), "max_request_bytes": maxRequestBytes,
		"max_response_bytes": maxResponseBytes, "max_text_bytes": inference.MaxTextBytes}
}

// Serve 持续处理连接中的请求直到 EOF、关闭请求、取消或通信失败。
// 连接上下文控制读取与请求的生命周期；返回后由创建者关闭执行管理器，
// 再关闭连接以排空终态事件，最后关闭网络与数据库资源。
func (s *Server) Serve(c *Connection) error {
	if c == nil {
		return errors.New("协议连接不能为空")
	}
	if err := c.notify("system.ready", s.info()); err != nil {
		return c.normalizeError(err)
	}
	scanner := bufio.NewScanner(c.in)
	scanner.Buffer(make([]byte, 4096), maxRequestBytes+2)
	for scanner.Scan() {
		if c.ctx.Err() != nil {
			return nil
		}
		line := scanner.Bytes()
		if len(line) > maxRequestBytes {
			return errors.New("协议请求超过 1 MiB 限制")
		}
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		stop, err := s.handle(c, line)
		if err != nil {
			return c.normalizeError(err)
		}
		if stop {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return c.normalizeError(fmt.Errorf("读取协议输入失败: %w", err))
	}
	return nil
}

func (s *Server) handle(c *Connection, line []byte) (bool, error) {
	req, parseErr := parseRequest(line)
	if parseErr != nil {
		return false, c.send(response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: parseErr})
	}
	// 配置操作有独立短期超时；不能将它作为异步模型执行的父上下文。
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()
	if req.Method == "model.invoke" {
		return false, s.invoke(ctx, c, req)
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
			result = map[string]any{"cancel_requested": s.runs.Cancel(p.RunID)}
		}
	default:
		result, rpcErr = s.catalog(ctx, req)
	}
	if rpcErr != nil && rpcErr.Code == -32603 {
		s.logger.Error("配置请求处理失败", "module", "stdio", "method", req.Method, "request_id", string(req.ID))
	}
	return stop, c.reply(req, result, rpcErr)
}
