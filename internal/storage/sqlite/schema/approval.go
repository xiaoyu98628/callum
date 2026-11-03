package schema

import "encoding/json"

// Approval 映射 conversation_run_approvals（执行审批表），保留用户决定及操作快照。
type Approval struct {
	ID           string          `gorm:"type:text;primaryKey;not null;check:ck_approval_id,id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-','')) = 32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'"`
	RunID        string          `gorm:"type:text;not null;index:ix_approval_run,priority:1"`
	StepID       string          `gorm:"type:text;not null;uniqueIndex:uq_approval_step"`
	Request      json.RawMessage `gorm:"type:text;serializer:json;not null;check:ck_approval_request,json_valid(request) AND json_type(request) = 'object'"`
	RequestHash  string          `gorm:"type:text;not null;check:ck_approval_hash,length(request_hash) = 64 AND request_hash NOT GLOB '*[^0-9a-f]*'"`
	Status       string          `gorm:"type:text;not null;default:pending;index:ix_approval_pending;check:ck_approval_status,status IN ('pending','approved','rejected','cancelled')"`
	DecidedBy    *string         `gorm:"type:text"`
	DecisionNote *string         `gorm:"type:text"`
	CreatedAt    int64           `gorm:"not null;autoCreateTime:false;index:ix_approval_run,priority:2"`
	UpdatedAt    int64           `gorm:"not null;autoUpdateTime:false"`
	DecidedAt    *int64          `gorm:"check:ck_approval_decision,(status = 'pending' AND decided_at IS NULL) OR (status <> 'pending' AND decided_at IS NOT NULL)"`
	Run          *Run            `gorm:"foreignKey:RunID;references:ID;constraint:OnDelete:RESTRICT"`
	Step         *Step           `gorm:"foreignKey:StepID,RunID;references:ID,RunID;constraint:OnDelete:RESTRICT"`
}

func (Approval) TableName() string { return "conversation_run_approvals" }
