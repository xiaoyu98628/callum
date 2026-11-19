package stdio

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/xiaoyu98628/callum/internal/stdio/protocol"
	"github.com/xiaoyu98628/callum/internal/stdio/transport"
)

func (s *Server) handle(c *transport.Connection, pending *networkRequests, line []byte) (bool, error) {
	req, parseErr := protocol.ParseRequest(line)
	if parseErr != nil {
		return false, c.Send(protocol.Response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: parseErr})
	}
	// 配置操作有独立短期超时；不能将它作为异步模型执行的父上下文。
	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()
	reply := func(result any, rpcErr *protocol.Error) error {
		if rpcErr != nil && rpcErr.Code == -32603 {
			s.logger.Error("请求处理失败", "module", "stdio", "method", req.Method, "request_id", string(req.ID))
		}
		return c.Reply(req, result, rpcErr)
	}
	if strings.HasPrefix(req.Method, "mcp.") && s.mcp != nil {
		if req.Method == "mcp.test" || req.Method == "mcp.oauth.start" {
			select {
			case pending.slots <- struct{}{}:
				pending.wg.Go(func() {
					defer func() { <-pending.slots }()
					ctx, cancel := context.WithTimeout(pending.ctx, 12*time.Second)
					defer cancel()
					value, err := s.mcp.Handle(ctx, pending.ctx, req)
					if replyErr := reply(value, err); replyErr != nil {
						c.Fail(replyErr)
					}
				})
				return false, nil
			default:
				return false, reply(nil, &protocol.Error{Code: 3017, Message: "多个 MCP 连接操作正在进行，请稍后重试"})
			}
		}
		value, err := s.mcp.Handle(ctx, pending.ctx, req)
		return false, reply(value, err)
	}
	if req.Method == "model.invoke" {
		return false, s.execution.Invoke(ctx, req, reply)
	}
	if strings.HasPrefix(req.Method, "conversation.") && s.conversation != nil {
		return false, s.conversation.Handle(ctx, req, reply)
	}
	if strings.HasPrefix(req.Method, "approval.") && s.approvals != nil {
		value, err := s.approvals.Handle(ctx, req)
		return false, reply(value, err)
	}
	if strings.HasPrefix(req.Method, "user_input.") && s.inputs != nil {
		value, err := s.inputs.Handle(ctx, req)
		return false, reply(value, err)
	}
	if strings.HasPrefix(req.Method, "settings.") && s.settings != nil {
		value, err := s.settings.Handle(ctx, req)
		return false, reply(value, err)
	}
	if req.Method == "skills.list" && s.skills != nil {
		value, err := s.skills.List(ctx, req)
		return false, reply(value, err)
	}
	var result any
	var rpcErr *protocol.Error
	stop := false
	switch req.Method {
	case "system.info", "system.shutdown":
		rpcErr = protocol.DecodeParams(req.Params, &struct{}{})
		if rpcErr == nil {
			result = s.info()
			stop = req.Method == "system.shutdown"
		}
	case "run.cancel":
		result, rpcErr = s.execution.Cancel(req)
	case "run.get", "run.list", "step.get", "step.list", "step.data":
		result, rpcErr = s.execution.Query(ctx, req)
	default:
		if strings.HasPrefix(req.Method, "workspace.") && s.workspace != nil {
			result, rpcErr = s.workspace.Handle(ctx, req)
		} else {
			result, rpcErr = s.catalog.Handle(ctx, req)
		}
	}
	return stop, reply(result, rpcErr)
}
