package conversation

import (
	"errors"

	"github.com/xiaoyu98628/callum/internal/model"
)

const (
	FollowUpQueued   = "queued"
	FollowUpSteering = "steering"
	FollowUpApplied  = "applied"
	FollowUpSent     = "sent"
	FollowUpRemoved  = "removed"
	FollowUpLimit    = 32
)

var (
	ErrFollowUpChanged = errors.New("后续消息或当前执行已变化，请刷新后重试")
	ErrFollowUpLimit   = errors.New("后续消息已达到存储或数量上限")
	ErrSteered         = errors.New("已收到新的引导，原工具调用未执行")
)

type FollowUp struct {
	ID             string        `json:"id"`
	ConversationID string        `json:"conversation_id"`
	ExpectedRunID  string        `json:"expected_run_id"`
	ModelID        string        `json:"model_id"`
	Status         string        `json:"status"`
	Content        []model.Block `json:"content"`
	Position       int64         `json:"position"`
	Revision       int64         `json:"revision"`
	AppliedStepID  *string       `json:"applied_step_id,omitempty"`
	Receipt        *Message      `json:"-"`
	CreatedAt      int64         `json:"created_at"`
}

type FollowUpInput struct {
	ConversationID string        `json:"conversation_id"`
	ExpectedRunID  string        `json:"expected_run_id"`
	ModelID        string        `json:"model_id,omitempty"`
	Mode           string        `json:"mode"`
	Content        []model.Block `json:"content"`
}

type FollowUpChange struct {
	ConversationID string  `json:"conversation_id"`
	ID             string  `json:"id"`
	Revision       int64   `json:"revision"`
	Action         string  `json:"action"`
	ExpectedRunID  string  `json:"expected_run_id,omitempty"`
	Text           *string `json:"text,omitempty"`
}

type FollowUpStart struct {
	ConversationID string `json:"conversation_id"`
	ID             string `json:"id"`
	Revision       int64  `json:"revision"`
	AfterRunID     string `json:"after_run_id,omitempty"`
}
