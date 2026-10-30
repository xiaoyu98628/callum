package stdio

import "github.com/xiaoyu98628/callum/internal/inference"

// Emit 将业务执行事件映射为协议通知，供装配层作为事件回调注入执行管理器。
// 本方法支持并发调用，错误描述在协议边界转换，原始错误链不参与序列化。
func (c *Connection) Emit(event inference.Event) error {
	params := struct {
		RunID             string    `json:"run_id"`
		Text              *string   `json:"text,omitempty"`
		ProviderRequestID string    `json:"provider_request_id,omitempty"`
		Error             *rpcError `json:"error,omitempty"`
	}{RunID: event.RunID, Text: event.Text, ProviderRequestID: event.ProviderRequestID}
	if event.Err != nil {
		params.Error = publicError(event.Err, true)
	}
	return c.notify("run."+event.Type, params)
}
