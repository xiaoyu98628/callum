package stdio

import (
	"context"
	"encoding/json"
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
	if req.Method == "model.invoke" {
		return false, s.execution.Invoke(ctx, req, func(result any, rpcErr *protocol.Error) error { return c.Reply(req, result, rpcErr) })
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
	default:
		result, rpcErr = s.catalog.Handle(ctx, req)
	}
	if rpcErr != nil && rpcErr.Code == -32603 {
		s.logger.Error("配置请求处理失败", "module", "stdio", "method", req.Method, "request_id", string(req.ID))
	}
	return stop, c.Reply(req, result, rpcErr)
}
