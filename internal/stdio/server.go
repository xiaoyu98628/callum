package stdio

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"log/slog"

	"github.com/xiaoyu98628/callum/internal/agent/approval"
	agentservice "github.com/xiaoyu98628/callum/internal/agent/service"
	conversationservice "github.com/xiaoyu98628/callum/internal/conversation/service"
	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/inference/executor"
	providerservice "github.com/xiaoyu98628/callum/internal/model/provider/service"
	approvalhandler "github.com/xiaoyu98628/callum/internal/stdio/approval"
	"github.com/xiaoyu98628/callum/internal/stdio/catalog"
	conversationhandler "github.com/xiaoyu98628/callum/internal/stdio/conversation"
	"github.com/xiaoyu98628/callum/internal/stdio/execution"
	"github.com/xiaoyu98628/callum/internal/stdio/transport"
	workspacehandler "github.com/xiaoyu98628/callum/internal/stdio/workspace"
	workspaceservice "github.com/xiaoyu98628/callum/internal/workspace/service"
)

// Server 只负责协议处理，配置业务服务与执行管理器均由装配层注入。
// 连接、任务、数据库及 HTTP 资源的创建与关闭顺序由调用方管理。
type Server struct {
	catalog      *catalog.Handler
	execution    *execution.Handler
	workspace    *workspacehandler.Handler
	conversation *conversationhandler.Handler
	logger       *slog.Logger
	approvals    *approvalhandler.Handler
}

// NewWithConversations 装配普通对话接口；所有服务和资源仍由 app 持有。
func NewWithConversations(providers *providerservice.Service, runs *executor.Manager, workspaces *workspaceservice.Service, conversations *conversationservice.Service, persistent *agentservice.Engine, approvals *approval.Service, accepted func(string) error, logger *slog.Logger) (*Server, error) {
	if workspaces == nil || conversations == nil || persistent == nil || approvals == nil || accepted == nil {
		return nil, errors.New("对话服务不能为空")
	}
	s, err := New(providers, runs, logger)
	if err != nil {
		return nil, err
	}
	s.approvals = approvalhandler.New(approvals)
	s.workspace = workspacehandler.New(workspaces)
	s.conversation = conversationhandler.New(conversations, accepted)
	s.execution = execution.NewPersistent(runs, persistent, accepted)
	return s, nil
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

// Ready 将就绪通知排入连接，必须在该连接的事件转发启动前调用。
func (s *Server) Ready(c *transport.Connection) error {
	if c == nil {
		return errors.New("协议连接不能为空")
	}
	return c.NormalizeError(c.Notify("system.ready", s.info()))
}

// Serve 发送就绪通知并处理请求，资源仍由创建者关闭。
func (s *Server) Serve(c *transport.Connection) error {
	if err := s.Ready(c); err != nil {
		return err
	}
	return s.ServeRequests(c)
}

// ServeRequests 持续处理请求直到 EOF、关闭请求、取消或通信失败。
// 返回后创建者应停止本连接的任务和事件转发，再排空并关闭连接。
func (s *Server) ServeRequests(c *transport.Connection) error {
	if c == nil {
		return errors.New("协议连接不能为空")
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
