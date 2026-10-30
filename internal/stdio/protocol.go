package stdio

import (
	"bytes"
	"encoding/json"
	"io"
	"unicode/utf8"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

func parseRequest(line []byte) (request, *rpcError) {
	var req request
	if !utf8.Valid(line) || !json.Valid(line) {
		return req, &rpcError{Code: -32700, Message: "JSON 格式无效"}
	}
	trimmed := bytes.TrimSpace(line)
	// 本协议按行传递单个请求或通知对象，批量数组不属于支持的通信格式。
	var fields map[string]json.RawMessage
	if len(trimmed) == 0 || trimmed[0] != '{' || json.Unmarshal(line, &fields) != nil ||
		json.Unmarshal(fields["jsonrpc"], &req.JSONRPC) != nil || json.Unmarshal(fields["method"], &req.Method) != nil || req.JSONRPC != "2.0" || req.Method == "" {
		return request{}, &rpcError{Code: -32600, Message: "请求对象无效"}
	}
	req.ID, req.Params = fields["id"], fields["params"]
	if req.ID != nil {
		var id any
		decoder := json.NewDecoder(bytes.NewReader(req.ID))
		decoder.UseNumber()
		if decoder.Decode(&id) != nil {
			return request{}, &rpcError{Code: -32600, Message: "请求 ID 无效"}
		}
		switch id.(type) {
		case string, json.Number, nil:
		default:
			return request{}, &rpcError{Code: -32600, Message: "请求 ID 无效"}
		}
	}
	return req, nil
}

func decodeParams(raw json.RawMessage, target any) *rpcError {
	if raw == nil {
		raw = json.RawMessage(`{}`)
	}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
		return invalidParams()
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		return invalidParams()
	}
	if decoder.Decode(new(any)) != io.EOF {
		return invalidParams()
	}
	return nil
}

func (c *Connection) reply(req request, result any, err *rpcError) error {
	if req.ID == nil {
		return nil
	}
	return c.send(response{JSONRPC: "2.0", ID: req.ID, Result: result, Error: err})
}

func (c *Connection) notify(method string, params any) error {
	return c.send(notification{JSONRPC: "2.0", Method: method, Params: params})
}
