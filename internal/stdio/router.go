package stdio

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/xiaoyu98628/callum/internal/stdio/protocol"
	"github.com/xiaoyu98628/callum/internal/stdio/transport"
)

func (s *Server) handle(c *transport.Connection, line []byte) (bool, error) {
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
	if req.Method == "model.invoke" {
		return false, s.execution.Invoke(ctx, req, reply)
	}
	if strings.HasPrefix(req.Method, "conversation.") && s.conversation != nil {
		return false, s.conversation.Handle(ctx, req, reply)
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
