package execution

import (
	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

// Emit 将业务执行事件映射为协议通知，供装配层作为事件回调注入执行管理器。
// 本方法支持并发调用，错误描述在协议边界转换，原始错误链不参与序列化。
func Emit(notify func(string, any) error, event inference.Event) error {
	params := struct {
		RunID             string          `json:"run_id"`
		Text              *string         `json:"text,omitempty"`
		ProviderRequestID string          `json:"provider_request_id,omitempty"`
		Error             *protocol.Error `json:"error,omitempty"`
	}{RunID: event.RunID, Text: event.Text, ProviderRequestID: event.ProviderRequestID}
	if event.Err != nil {
		params.Error = publicError(event.Err)
	}
	return notify("run."+event.Type, params)
}
