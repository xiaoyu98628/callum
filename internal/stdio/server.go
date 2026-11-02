package stdio

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"log/slog"

	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/inference/executor"
	providerservice "github.com/xiaoyu98628/callum/internal/model/provider/service"
	"github.com/xiaoyu98628/callum/internal/stdio/catalog"
	"github.com/xiaoyu98628/callum/internal/stdio/execution"
	"github.com/xiaoyu98628/callum/internal/stdio/transport"
)

// Server 只负责协议处理，配置业务服务与执行管理器均由装配层注入。
// 连接、任务、数据库及 HTTP 资源的创建与关闭顺序由调用方管理。
type Server struct {
	catalog   *catalog.Handler
	execution *execution.Handler
	logger    *slog.Logger
}

// New 创建协议处理对象，不创建或关闭外部资源。
func New(providers *providerservice.Service, runs *executor.Manager, logger *slog.Logger) (*Server, error) {
	if providers == nil || runs == nil || logger == nil {
		return nil, errors.New("通信服务初始化参数无效")
	}
	return &Server{catalog: catalog.New(providers), execution: execution.New(runs), logger: logger}, nil
}

func (s *Server) info() map[string]any {
	limit, timeout := s.execution.Limits()
	return map[string]any{"service": "callum", "protocol_version": 1, "max_concurrent_runs": limit,
		"call_timeout_ms": timeout.Milliseconds(), "max_request_bytes": transport.MaxRequestBytes,
		"max_response_bytes": transport.MaxResponseBytes, "max_text_bytes": inference.MaxTextBytes}
}

// Serve 持续处理连接中的请求直到 EOF、关闭请求、取消或通信失败。
// 连接上下文控制读取与请求的生命周期；返回后由创建者关闭执行管理器，
// 再关闭连接以排空终态事件，最后关闭网络与数据库资源。
func (s *Server) Serve(c *transport.Connection) error {
	if c == nil {
		return errors.New("协议连接不能为空")
	}
	if err := c.Notify("system.ready", s.info()); err != nil {
		return c.NormalizeError(err)
	}
	scanner := bufio.NewScanner(c.Input())
	scanner.Buffer(make([]byte, 4096), transport.MaxRequestBytes+2)
	for scanner.Scan() {
		if c.Context().Err() != nil {
			return nil
		}
		line := scanner.Bytes()
		if len(line) > transport.MaxRequestBytes {
			return errors.New("协议请求超过 1 MiB 限制")
		}
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		stop, err := s.handle(c, line)
		if err != nil {
			return c.NormalizeError(err)
		}
		if stop {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return c.NormalizeError(fmt.Errorf("读取协议输入失败: %w", err))
	}
	return nil
}
