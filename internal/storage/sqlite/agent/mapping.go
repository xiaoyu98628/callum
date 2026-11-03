package agent

import (
	domain "github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

func step(row schema.Step) domain.Step {
	return domain.Step{ID: row.ID, ConversationID: row.ConversationID, RunID: row.RunID, ParentStepID: row.ParentStepID, Sequence: row.Sequence, Type: row.Type, Name: row.Name, Definition: row.Definition, Status: row.Status, Input: row.Input, Output: row.Output, Usage: row.Usage, Model: row.Model, ProviderRequestID: row.ProviderRequestID, ToolCallID: row.ToolCallID, ErrorKind: row.ErrorKind, ErrorMessage: row.ErrorMessage, CreatedAt: row.CreatedAt, StartedAt: row.StartedAt, UpdatedAt: row.UpdatedAt, FinishedAt: row.FinishedAt}
}
func record(in domain.Step) schema.Step {
	return schema.Step{ID: in.ID, ConversationID: in.ConversationID, RunID: in.RunID, ParentStepID: in.ParentStepID, Sequence: in.Sequence, Type: in.Type, Name: in.Name, Definition: in.Definition, Status: in.Status, Input: in.Input, Output: in.Output, Usage: in.Usage, Model: in.Model, ProviderRequestID: in.ProviderRequestID, ToolCallID: in.ToolCallID, ErrorKind: in.ErrorKind, ErrorMessage: in.ErrorMessage, CreatedAt: in.CreatedAt, StartedAt: in.StartedAt, UpdatedAt: in.UpdatedAt, FinishedAt: in.FinishedAt}
}
func execution(row schema.Run) domain.Run {
	return domain.Run{ID: row.ID, ConversationID: row.ConversationID, ReplacesRunID: row.ReplacesRunID, Model: row.Model, Stream: row.Stream, Status: row.Status, ErrorKind: row.ErrorKind, ErrorMessage: row.ErrorMessage, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, FinishedAt: row.FinishedAt}
}
