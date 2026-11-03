package execution

import (
	"github.com/xiaoyu98628/callum/internal/inference"
)

// Emit 将业务执行事件映射为协议通知，供装配层作为事件回调注入执行管理器。
// 本方法支持并发调用，错误描述在协议边界转换，原始错误链不参与序列化。
func Emit(notify func(string, any) error, event inference.Event) error {
	return EmitPersistent(notify, inference.PublicEvent(event))
}
