package protocol

import (
	"bytes"
	"encoding/json"
	"io"
	"unicode/utf8"
)

// Request 是解析后的单个 JSON-RPC 请求；ID 为 nil 表示通知。
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// ParseRequest 校验逐行 JSON-RPC 请求，不接受批量请求。
func ParseRequest(line []byte) (Request, *Error) {
	var req Request
	if !utf8.Valid(line) || !json.Valid(line) {
		return req, &Error{Code: -32700, Message: "JSON 格式无效"}
	}
	trimmed := bytes.TrimSpace(line)
	// 本协议按行传递单个请求或通知对象，批量数组不属于支持的通信格式。
	var fields map[string]json.RawMessage
	if len(trimmed) == 0 || trimmed[0] != '{' || json.Unmarshal(line, &fields) != nil ||
		json.Unmarshal(fields["jsonrpc"], &req.JSONRPC) != nil || json.Unmarshal(fields["method"], &req.Method) != nil || req.JSONRPC != "2.0" || req.Method == "" {
		return Request{}, &Error{Code: -32600, Message: "请求对象无效"}
	}
	req.ID, req.Params = fields["id"], fields["params"]
	if req.ID != nil {
		var id any
		decoder := json.NewDecoder(bytes.NewReader(req.ID))
		decoder.UseNumber()
		if decoder.Decode(&id) != nil {
			return Request{}, &Error{Code: -32600, Message: "请求 ID 无效"}
		}
		switch id.(type) {
		case string, json.Number, nil:
		default:
			return Request{}, &Error{Code: -32600, Message: "请求 ID 无效"}
		}
	}
	return req, nil
}

// DecodeParams 解析具名参数对象，拒绝未知字段和无效类型。
func DecodeParams(raw json.RawMessage, target any) *Error {
	if raw == nil {
		raw = json.RawMessage(`{}`)
	}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
		return InvalidParams()
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		return InvalidParams()
	}
	if decoder.Decode(new(any)) != io.EOF {
		return InvalidParams()
	}
	return nil
}
