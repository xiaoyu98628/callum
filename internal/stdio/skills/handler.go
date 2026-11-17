package skills

import (
	"context"
	"encoding/json"

	"github.com/xiaoyu98628/callum/internal/skills"
	"github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

// Handler 将只读技能目录转换为协议响应，目录由运行时持有。
type Handler struct{ store *skills.Store }

// New 接收运行时已经打开的非 nil 技能目录。
func New(store *skills.Store) *Handler { return &Handler{store: store} }

// List 只读取已发现技能的元数据。
func (h *Handler) List(ctx context.Context, req protocol.Request) (json.RawMessage, *protocol.Error) {
	if err := protocol.DecodeParams(req.Params, &struct{}{}); err != nil {
		return nil, err
	}
	data, err := h.store.Catalog(ctx)
	if err != nil {
		return nil, &protocol.Error{Code: -32603, Message: "无法读取技能列表"}
	}
	return data, nil
}
