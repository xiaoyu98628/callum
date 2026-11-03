package schema

import (
	"encoding/json"

	"github.com/xiaoyu98628/callum/internal/model"
)

// Step 映射 conversation_run_steps（执行步骤表），只在存储层使用。
type Step struct {
	ID                string          `gorm:"type:text;primaryKey;not null;uniqueIndex:uq_step_run,priority:1;uniqueIndex:uq_step_identity,priority:1;check:ck_step_id,id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-','')) = 32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'"`
	ConversationID    string          `gorm:"type:text;not null;uniqueIndex:uq_step_identity,priority:3"`
	RunID             string          `gorm:"type:text;not null;uniqueIndex:uq_step_identity,priority:2;uniqueIndex:uq_step_run,priority:2;uniqueIndex:uq_step_sequence,priority:1"`
	ParentStepID      *string         `gorm:"type:text;check:ck_step_parent,parent_step_id IS NULL OR parent_step_id <> id"`
	Sequence          int64           `gorm:"not null;uniqueIndex:uq_step_sequence,priority:2;check:ck_step_sequence,sequence > 0"`
	Type              string          `gorm:"type:text;not null;check:ck_step_type,type IN ('model_call','tool_call','user_input','context_compression')"`
	Name              string          `gorm:"type:text;not null;check:ck_step_name,length(trim(name)) > 0"`
	Definition        json.RawMessage `gorm:"type:text;serializer:json;check:ck_step_definition,definition IS NULL OR json_valid(definition)"`
	Status            string          `gorm:"type:text;not null;check:ck_step_status,status IN ('pending','running','awaiting_approval','awaiting_user_input','completed','failed','cancelled','interrupted')"`
	Input             json.RawMessage `gorm:"type:text;serializer:json;not null;default:('{}');check:ck_step_input,json_valid(input) AND json_type(input) = 'object'"`
	Output            json.RawMessage `gorm:"type:text;serializer:json;check:ck_step_output,output IS NULL OR json_valid(output)"`
	Usage             json.RawMessage `gorm:"type:text;serializer:json;check:ck_step_usage,usage IS NULL OR (json_valid(usage) AND json_type(usage) = 'object')"`
	Model             *model.Snapshot `gorm:"type:text;serializer:json;check:ck_step_model,model IS NULL OR (json_valid(model) AND json_type(model) = 'object')"`
	ProviderRequestID *string         `gorm:"type:text"`
	ToolCallID        *string         `gorm:"type:text"`
	ErrorKind         *string         `gorm:"type:text"`
	ErrorMessage      *string         `gorm:"type:text"`
	CreatedAt         int64           `gorm:"not null;autoCreateTime:false"`
	StartedAt         *int64
	UpdatedAt         int64  `gorm:"not null;autoUpdateTime:false"`
	FinishedAt        *int64 `gorm:"check:ck_step_finished,(status IN ('pending','running','awaiting_approval','awaiting_user_input') AND finished_at IS NULL) OR (status IN ('completed','failed','cancelled','interrupted') AND finished_at IS NOT NULL)"`
	Run               *Run   `gorm:"foreignKey:RunID,ConversationID;references:ID,ConversationID;constraint:OnDelete:RESTRICT"`
	Parent            *Step  `gorm:"foreignKey:ParentStepID,RunID,ConversationID;references:ID,RunID,ConversationID;constraint:OnDelete:RESTRICT"`
}

func (Step) TableName() string { return "conversation_run_steps" }
