package agent

import (
	"context"
	"encoding/json"

	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/settings"
)

const (
	Pending     = "pending"
	Running     = "running"
	Completed   = "completed"
	Failed      = "failed"
	Cancelled   = "cancelled"
	Interrupted = "interrupted"
)

// Run 保存一次持久化执行的状态，资源由执行调度器管理。
type Run struct {
	ID               string             `json:"id"`
	ConversationID   string             `json:"conversation_id"`
	ReplacesRunID    *string            `json:"replaces_run_id"`
	Model            model.Snapshot     `json:"model"`
	Stream           bool               `json:"stream"`
	SettingsSnapshot *settings.Snapshot `json:"settings_snapshot"`
	Status           string             `json:"status"`
	ErrorKind        *string            `json:"error_kind"`
	ErrorMessage     *string            `json:"error_message"`
	CreatedAt        int64              `json:"created_at"`
	UpdatedAt        int64              `json:"updated_at"`
	FinishedAt       *int64             `json:"finished_at"`
}

// Step 保存一次实际操作；模型响应和 usage 使用 JSON 保留协议字段。
type Step struct {
	ID                string          `json:"id"`
	ConversationID    string          `json:"conversation_id"`
	RunID             string          `json:"run_id"`
	ParentStepID      *string         `json:"parent_step_id"`
	Sequence          int64           `json:"sequence"`
	Type              string          `json:"type"`
	Name              string          `json:"name"`
	Definition        json.RawMessage `json:"definition"`
	Status            string          `json:"status"`
	Input             json.RawMessage `json:"input"`
	Output            json.RawMessage `json:"output"`
	Usage             json.RawMessage `json:"usage"`
	Model             *model.Snapshot `json:"model"`
	ProviderRequestID *string         `json:"provider_request_id"`
	ToolCallID        *string         `json:"tool_call_id"`
	ErrorKind         *string         `json:"error_kind"`
	ErrorMessage      *string         `json:"error_message"`
	CreatedAt         int64           `json:"created_at"`
	StartedAt         *int64          `json:"started_at"`
	UpdatedAt         int64           `json:"updated_at"`
	FinishedAt        *int64          `json:"finished_at"`
}

// Event 是与通信协议无关的业务通知，Data 及其引用内容在发布后只读。
type Event struct {
	Type              string
	ConversationID    string
	RunID             string
	StepID            string
	MessageID         string
	Data              any
	RemovedMessageIDs []string
	StatePersisted    *bool
}

// Resolved 是单次执行的配置快照，Client 的 HTTP 资源仍由装配层管理。
type Resolved struct {
	Client     model.Client
	Snapshot   model.Snapshot
	Protocol   string
	ImageInput bool
}

type Resolver func(context.Context, string) (Resolved, error)
