package execution

import (
	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

// Error 为对话通信边界复用模型调用的受控错误转换，不暴露底层错误链。
func Error(err error) *protocol.Error {
	if err == nil {
		return nil
	}
	return publicError(err)
}

func publicError(err error) *protocol.Error { return protocolError(inference.DescribeError(err)) }

// protocolError 仅在协议边界分配 JSON-RPC 错误码。
func protocolError(f inference.Failure) *protocol.Error {
	if f.Kind == "invalid_params" {
		return protocol.InvalidParams()
	}
	code := 1009
	switch f.Kind {
	case "internal":
		code = -32603
	case "not_found":
		code = 1001
	case "conflict":
		code = 1002
	case "referenced":
		code = 1003
	case "disabled":
		code = 1004
	case "unsupported":
		code = 1005
	case "busy":
		code = 1006
	case "cancelled":
		code = 1007
	case "timeout":
		code = 1008
	case "output_limit":
		code = 1010
	case "closing":
		code = 1011
	case "credentials_unavailable":
		code = 1012
	}
	data := map[string]any{"kind": f.Kind}
	if f.ProviderRequestID != "" {
		data["provider_request_id"] = f.ProviderRequestID
	}
	if f.HTTPStatus != 0 {
		data["http_status"] = f.HTTPStatus
	}
	return &protocol.Error{Code: code, Message: f.Message, Data: data}
}
