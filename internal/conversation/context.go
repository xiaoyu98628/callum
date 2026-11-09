package conversation

import (
	"context"
	"encoding/json"
	"slices"
)

// ContextSource 固定原始消息和对应工具轨迹；指纹不包含凭据。
type ContextSource struct {
	MessageID   string `json:"message_id"`
	RunID       string `json:"run_id"`
	Sequence    int64  `json:"sequence"`
	Fingerprint string `json:"fingerprint"`
}

// ContextVersion 是可追溯的历史摘要。原文保留，只有 active 版本参与模型请求。
type ContextVersion struct {
	ID                string          `json:"id"`
	ConversationID    string          `json:"conversation_id"`
	Version           int64           `json:"version"`
	BaseVersionID     *string         `json:"base_version_id"`
	GeneratedByStepID string          `json:"generated_by_step_id"`
	ThroughSequence   int64           `json:"through_sequence"`
	Summary           string          `json:"summary,omitempty"`
	Sources           []ContextSource `json:"source_manifest,omitempty"`
	Stats             json.RawMessage `json:"size_stats,omitempty"`
	Status            string          `json:"status"`
	CreatedAt         int64           `json:"created_at"`
	UpdatedAt         int64           `json:"updated_at"`
	ActivatedAt       *int64          `json:"activated_at"`
	InvalidatedAt     *int64          `json:"invalidated_at"`
}

// Compact 不创建用户消息；ModelID 省略时使用会话当前模型。
type Compact struct {
	ConversationID string `json:"conversation_id"`
	ModelID        string `json:"model_id,omitempty"`
}

// ContextRepository 管理摘要生命周期；生成模型输出期间不持有事务。
type ContextRepository interface {
	ContextState(context.Context, string) (ContextState, error)
	ContextPage(context.Context, string, int64, int64) ([]Message, bool, error)
	LoadContextHistory(context.Context, string, int64) ([]Message, *ContextVersion, bool, error)
	ActiveContext(context.Context, string) (*ContextVersion, error)
	ContextVersion(context.Context, string, string) (ContextVersion, error)
	ContextVersions(context.Context, string, int, int) ([]ContextVersion, error)
	BeginCompaction(context.Context, Begin) (Execution, error)
	CreateContext(context.Context, ContextVersion) (ContextVersion, error)
	ActivateContext(context.Context, ContextVersion) error
	FailContext(context.Context, string) error
}

// Clone 隔离受理回调借用的版本数据，防止修改真实执行的摘要和来源。
func (v ContextVersion) Clone() ContextVersion {
	v.Sources = slices.Clone(v.Sources)
	v.Stats = slices.Clone(v.Stats)
	if v.BaseVersionID != nil {
		v.BaseVersionID = new(*v.BaseVersionID)
	}
	if v.ActivatedAt != nil {
		v.ActivatedAt = new(*v.ActivatedAt)
	}
	if v.InvalidatedAt != nil {
		v.InvalidatedAt = new(*v.InvalidatedAt)
	}
	return v
}
