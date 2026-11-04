package agent_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	store "github.com/xiaoyu98628/callum/internal/storage/sqlite/agent"
	conversationstore "github.com/xiaoyu98628/callum/internal/storage/sqlite/conversation"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	"github.com/xiaoyu98628/callum/internal/tool"
)

func contextOutput(t *testing.T, message model.Message, fields map[string]any) json.RawMessage {
	t.Helper()
	if fields == nil {
		fields = map[string]any{}
	}
	fields["context_message"] = message
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func operationFixture(t *testing.T, versioned bool) (*sqlite.Database, *store.Repository, conversation.Execution, agent.Step) {
	t.Helper()
	db, r, ex, step := fixture(t)
	step.ToolCallID = new("call-a")
	if err := db.DB.Model(&schema.Step{}).Where("id=?", step.ID).Update("tool_call_id", *step.ToolCallID).Error; err != nil {
		t.Fatal(err)
	}
	step.Definition = json.RawMessage(`{"effects":["write"]}`)
	if versioned {
		step.Input = json.RawMessage(`{"operation_version":1,"request_hash":"fixed-operation"}`)
	}
	if err := r.SaveStep(t.Context(), step); err != nil {
		t.Fatal(err)
	}
	parent := ex.Step
	parent.Status = agent.Completed
	parent.FinishedAt = new(int64(2))
	parent.Output = contextOutput(t, model.Message{Role: model.RoleAssistant, Content: "尚未完成的说明", ToolCalls: []model.ToolCall{{ID: "call-a", Name: "operation", Arguments: json.RawMessage(`{}`)}, {ID: "call-b", Name: "operation", Arguments: json.RawMessage(`{}`)}}}, nil)
	if err := r.SaveStep(t.Context(), parent); err != nil {
		t.Fatal(err)
	}
	return db, r, ex, step
}

func saveOperation(t *testing.T, r *store.Repository, step agent.Step, outcome tool.Outcome, versioned bool) {
	t.Helper()
	fields := map[string]any{"result": map[string]any{"written": true}}
	if versioned {
		fields["operation"] = tool.OutcomeRecord{Version: 1, Outcome: outcome, RequestHash: "fixed-operation"}
	}
	body, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	step.Output = contextOutput(t, model.Message{Role: model.RoleTool, ToolCallID: *step.ToolCallID, Content: string(body)}, fields)
	step.Status = agent.Completed
	step.StartedAt, step.FinishedAt = new(int64(2)), new(int64(3))
	if err := r.SaveStep(t.Context(), step); err != nil {
		t.Fatal(err)
	}
}

func finishOperationRun(t *testing.T, r *store.Repository, ex conversation.Execution, status string) {
	t.Helper()
	ex.Run.Status = status
	ex.Run.UpdatedAt = time.Now().UnixMilli()
	ex.Run.FinishedAt = new(ex.Run.UpdatedAt)
	if err := r.Finish(t.Context(), ex.Run); err != nil {
		t.Fatal(err)
	}
}

func TestFailedRunsRestorePairedEffectsWithoutFailedAnswer(t *testing.T) {
	for _, status := range []string{agent.Failed, agent.Cancelled, agent.Interrupted} {
		for _, outcome := range []tool.Outcome{tool.OutcomeApplied, tool.OutcomePartial, tool.OutcomeUnknown} {
			t.Run(status+"_"+string(outcome), func(t *testing.T) {
				_, r, ex, step := operationFixture(t, true)
				saveOperation(t, r, step, outcome, true)
				finishOperationRun(t, r, ex, status)
				transcript, err := r.Transcript(t.Context(), ex.Conversation.ID)
				if err != nil {
					t.Fatal(err)
				}
				messages := transcript[ex.Run.ID]
				if len(messages) != 3 || messages[0].Content != "" || len(messages[0].ToolCalls) != 2 || messages[1].ToolCallID != "call-a" || messages[2].ToolCallID != "call-b" {
					t.Fatalf("事实或配对丢失: %+v", messages)
				}
				var actual, notStarted struct {
					Operation tool.OutcomeRecord `json:"operation"`
					Result    struct {
						Written bool `json:"written"`
					} `json:"result"`
				}
				if json.Unmarshal([]byte(messages[1].Content), &actual) != nil || actual.Operation.Outcome != outcome || !actual.Result.Written {
					t.Fatalf("实际输出丢失: %s", messages[1].Content)
				}
				if json.Unmarshal([]byte(messages[2].Content), &notStarted) != nil || notStarted.Operation.Outcome != tool.OutcomeNotApplied {
					t.Fatalf("未执行调用被错误恢复: %s", messages[2].Content)
				}
			})
		}
	}
}

func TestRecoverySettlesOnlyNewUnfinishedOperations(t *testing.T) {
	for _, action := range []string{"recover", "finish"} {
		for _, test := range []struct {
			name               string
			started, versioned bool
			checkpoint         tool.Outcome
			want               tool.Outcome
		}{
			{"pending", false, true, "", tool.OutcomeNotApplied},
			{"started", true, true, "", tool.OutcomeUnknown},
			{"checkpoint", true, true, tool.OutcomeUnknown, tool.OutcomeUnknown},
			{"known_applied", true, true, tool.OutcomeApplied, tool.OutcomeApplied},
			{"legacy", true, false, "", ""},
		} {
			t.Run(action+"_"+test.name, func(t *testing.T) {
				_, r, ex, step := operationFixture(t, test.versioned)
				if test.started {
					var err error
					step, err = r.StartTool(t.Context(), ex.Run.ID, step.ID, "", "fixed-operation")
					if err != nil {
						t.Fatal(err)
					}
				}
				if test.checkpoint != "" {
					step.Output = contextOutput(t, model.Message{Role: model.RoleTool, ToolCallID: "call-a", Content: `{}`}, map[string]any{"operation": tool.OutcomeRecord{Version: 1, Outcome: test.checkpoint, RequestHash: "fixed-operation"}, "result": map[string]bool{"written": true}})
					if err := r.SaveStep(t.Context(), step); err != nil {
						t.Fatal(err)
					}
				}
				if action == "recover" {
					if err := r.Recover(t.Context()); err != nil {
						t.Fatal(err)
					}
				} else {
					finishOperationRun(t, r, ex, agent.Cancelled)
				}
				saved, err := r.GetStep(t.Context(), step.ID)
				if err != nil {
					t.Fatal(err)
				}
				if !test.versioned {
					if len(saved.Output) != 0 {
						t.Fatalf("替旧记录推断了事实: %s", saved.Output)
					}
				} else {
					var value struct {
						Operation tool.OutcomeRecord `json:"operation"`
						Result    json.RawMessage    `json:"result"`
					}
					if json.Unmarshal(saved.Output, &value) != nil || value.Operation.Outcome != test.want || !value.Operation.Recovered {
						t.Fatalf("恢复事实错误: %s", saved.Output)
					}
					if test.checkpoint != "" && !strings.Contains(string(value.Result), `"written":true`) {
						t.Fatal("恢复覆盖了实际输出")
					}
				}
				transcript, err := r.Transcript(t.Context(), ex.Conversation.ID)
				if err != nil {
					t.Fatal(err)
				}
				if (len(transcript[ex.Run.ID]) > 0) != test.want.MayHaveEffects() {
					t.Fatalf("恢复上下文范围错误: %+v", transcript)
				}
				// 重复恢复不修改已结束记录。
				if err := r.Recover(t.Context()); err != nil {
					t.Fatal(err)
				}
				again, err := r.GetStep(t.Context(), step.ID)
				if err != nil || string(again.Output) != string(saved.Output) {
					t.Fatalf("重复恢复改变事实: %v", err)
				}
			})
		}
	}
}

func TestLegacyFailureAndResentAuditDoNotEnterContext(t *testing.T) {
	for _, legacy := range []bool{true, false} {
		t.Run(map[bool]string{true: "legacy_failure", false: "resent_audit"}[legacy], func(t *testing.T) {
			db, r, ex, step := operationFixture(t, !legacy)
			saveOperation(t, r, step, tool.OutcomeApplied, !legacy)
			finishOperationRun(t, r, ex, agent.Failed)
			if !legacy {
				conversations, err := conversationstore.NewRepository(db.DB)
				if err != nil {
					t.Fatal(err)
				}
				_, err = conversations.Begin(t.Context(), conversation.Begin{ConversationID: ex.Conversation.ID, RunID: newID(t), UserMessageID: newID(t), AssistantMessageID: newID(t), StepID: newID(t), ResendMessageID: ex.User.ID, Model: ex.Run.Model, Protocol: "chatcompletions", Content: []model.Block{{Type: "text", Text: "更正问题"}}})
				if err != nil {
					t.Fatal(err)
				}
			}
			transcript, err := r.Transcript(t.Context(), ex.Conversation.ID)
			if err != nil || len(transcript) != 0 {
				t.Fatalf("旧失败或重发分支进入上下文: %+v %v", transcript, err)
			}
			// 审计步骤与真实操作事实仍保存在数据库中。
			saved, err := r.GetStep(t.Context(), step.ID)
			if err != nil || !strings.Contains(string(saved.Output), `"written":true`) {
				t.Fatalf("审计被删除: %s %v", saved.Output, err)
			}
		})
	}
}

func TestNewCorruptOutcomeBlocksContext(t *testing.T) {
	for _, output := range []string{
		`{"operation":{"version":1,"outcome":"invalid"}}`,
		`{"operation":{"version":1,"outcome":"applied"}}`,
	} {
		t.Run(output, func(t *testing.T) {
			db, r, ex, step := operationFixture(t, true)
			saveOperation(t, r, step, tool.OutcomeApplied, true)
			finishOperationRun(t, r, ex, agent.Failed)
			if err := db.DB.Model(&schema.Step{}).Where("id=?", step.ID).Update("output", output).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := r.Transcript(t.Context(), ex.Conversation.ID); !errors.Is(err, model.ErrInvalidResponse) {
				t.Fatalf("静默忽略损坏事实或缺失的结果配对: %v", err)
			}
		})
	}
}

func TestSuccessfulLegacyAndNewTranscriptsRemainCompatible(t *testing.T) {
	for _, versioned := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "versioned"}[versioned], func(t *testing.T) {
			_, r, ex, first := operationFixture(t, versioned)
			saveOperation(t, r, first, tool.OutcomeApplied, versioned)
			second := first
			second.ID, second.ToolCallID, second.Status, second.FinishedAt = newID(t), new("call-b"), agent.Pending, nil
			second, err := r.CreateStep(t.Context(), second)
			if err != nil {
				t.Fatal(err)
			}
			saveOperation(t, r, second, tool.OutcomeApplied, versioned)
			finishOperationRun(t, r, ex, agent.Completed)
			transcript, err := r.Transcript(t.Context(), ex.Conversation.ID)
			if err != nil || len(transcript[ex.Run.ID]) != 3 || transcript[ex.Run.ID][0].Content != "尚未完成的说明" {
				t.Fatalf("成功轨迹被改变: %+v %v", transcript, err)
			}
		})
	}
}

func TestTranscriptRejectsOversizedStoredContext(t *testing.T) {
	db, r, ex, step := operationFixture(t, true)
	saveOperation(t, r, step, tool.OutcomeApplied, true)
	finishOperationRun(t, r, ex, agent.Failed)
	large := contextOutput(t, model.Message{Role: model.RoleAssistant, Content: strings.Repeat("x", 8<<20)}, nil)
	if err := db.DB.Model(&schema.Step{}).Where("id=?", ex.Step.ID).Update("output", string(large)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := r.Transcript(t.Context(), ex.Conversation.ID); !errors.Is(err, model.ErrInputLimit) {
		t.Fatalf("历史轨迹绕过大小限制: %v", err)
	}
}
