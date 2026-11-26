package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/events"
	"github.com/xiaoyu98628/callum/internal/conversation"
	conversationservice "github.com/xiaoyu98628/callum/internal/conversation/service"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/settings"
	conversationstore "github.com/xiaoyu98628/callum/internal/storage/sqlite/conversation"
	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/policy"
)

func TestAutoReviewIncludesAppliedSteering(t *testing.T) {
	for _, scenario := range []string{"normal", "resume", "during_review"} {
		t.Run(scenario, func(t *testing.T) {
			resume := scenario == "resume"
			rt, _ := runtimeFixture(t, 1)
			record, err := rt.Settings().Get(t.Context(), settings.ExecutionKey)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(settings.Execution{ApprovalPolicy: policy.OnRequest, ApprovalsReviewer: policy.AutoReview, SandboxMode: tool.SandboxReadOnly})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := rt.Settings().Update(t.Context(), settings.ExecutionKey, record.Revision, raw); err != nil {
				t.Fatal(err)
			}
			project, err := rt.Workspaces().Create(t.Context(), "审批引导", t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			const guide = "现在仅允许创建 new.txt，不再创建 old.txt"
			var phase atomic.Int32
			var mainGuided, reviewGuided, reviews atomic.Int32
			var skipped atomic.Bool
			var svc *conversationservice.Service
			var execution conversation.Execution
			client := outcomeTestModel(func(_ context.Context, req model.Request) (model.Response, error) {
				if strings.HasPrefix(req.Messages[0].Content, "你独立审核") {
					reviews.Add(1)
					if scenario == "during_review" {
						_, err := svc.AddFollowUp(t.Context(), conversation.FollowUpInput{ConversationID: execution.Conversation.ID, ExpectedRunID: execution.Run.ID, Mode: "steer", Content: []model.Block{{Type: "text", Text: guide}}})
						return model.Response{Text: `{"decision":"approved","reason":"受控测试批准旧操作"}`}, err
					}
					if strings.Contains(req.Messages[1].Content, guide) {
						reviewGuided.Add(1)
					}
					return model.Response{Text: `{"decision":"rejected","reason":"受控测试拒绝执行"}`}, nil
				}
				if len(req.Tools) == 0 {
					return model.Response{Text: "审批引导"}, nil
				}
				callID, name := "initial-patch", "new.txt"
				if phase.Load() == 1 {
					callID, name = "resumed-patch", "new-resumed.txt"
				}
				for _, m := range req.Messages {
					if strings.Contains(m.Content, guide) {
						mainGuided.Add(1)
					}
					for _, b := range m.Blocks {
						if strings.Contains(b.Text, guide) {
							mainGuided.Add(1)
						}
					}
				}
				for _, m := range req.Messages {
					if m.Role == model.RoleTool && m.ToolCallID == callID {
						if strings.Contains(m.Content, `"kind":"steered"`) {
							skipped.Store(true)
						}
						if resume && phase.Load() == 0 {
							return model.Response{}, context.Canceled
						}
						return model.Response{Text: "已结束"}, nil
					}
				}
				args, err := json.Marshal(map[string]any{"patch": "*** Begin Patch\n*** Add File: " + name + "\n+受控内容\n*** End Patch\n", "base_versions": []any{}})
				return model.Response{ToolCalls: []model.ToolCall{{ID: callID, Name: "apply_patch", Arguments: args}}}, err
			})
			repo, err := conversationstore.NewRepository(rt.db.DB)
			if err != nil {
				t.Fatal(err)
			}
			id := policyTestID(t)
			svc, err = conversationservice.New(repo, func(context.Context, string) (agent.Resolved, error) {
				return agent.Resolved{Client: client, Snapshot: model.Snapshot{ID: id, RemoteModelID: "controlled"}, Protocol: "chatcompletions", ContextWindow: 1000000}, nil
			}, rt.manager, rt.engine, rt.attachments, rt.skills, rt.events.Publish, rt.logger)
			if err != nil {
				t.Fatal(err)
			}
			sub, err := rt.Subscribe(events.Filter{})
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			err = svc.Send(t.Context(), conversation.Send{ModelID: id, WorkspaceID: &project.ID, Stream: new(false), Content: []model.Block{{Type: "text", Text: "请创建 old.txt"}}}, func(ex conversation.Execution) error {
				execution = ex
				if scenario == "during_review" {
					return nil
				}
				_, err := svc.AddFollowUp(t.Context(), conversation.FollowUpInput{ConversationID: ex.Conversation.ID, ExpectedRunID: ex.Run.ID, Mode: "steer", Content: []model.Block{{Type: "text", Text: guide}}})
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			terminal := awaitOutcome(t, rt, sub, execution.Run.ID, nil)
			wantReviews := int32(1)
			if resume {
				if terminal.Type != "run.cancelled" {
					t.Fatalf("执行没有停止: %+v", terminal)
				}
				phase.Store(1)
				if err := svc.Continue(t.Context(), conversation.Continue{ConversationID: execution.Conversation.ID, RunID: execution.Run.ID, ModelID: id}, func(conversation.Execution) error { return nil }); err != nil {
					t.Fatal(err)
				}
				terminal = awaitOutcome(t, rt, sub, execution.Run.ID, nil)
				wantReviews = 2
			}
			wantGuided := wantReviews
			if scenario == "during_review" {
				wantGuided = 0
				if !skipped.Load() {
					t.Fatal("审核期间出现新引导后，旧操作没有被跳过")
				}
			}
			if terminal.Type != "run.completed" || mainGuided.Load() == 0 || reviews.Load() != wantReviews || reviewGuided.Load() != wantGuided {
				t.Fatalf("审批未使用生效引导: terminal=%s main=%d reviews=%d", terminal.Type, mainGuided.Load(), reviewGuided.Load())
			}
			for _, name := range []string{"old.txt", "new.txt", "new-resumed.txt"} {
				if _, err := os.Stat(filepath.Join(project.RootPath, name)); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("被拒绝操作产生了文件: %s %v", name, err)
				}
			}
		})
	}
}
