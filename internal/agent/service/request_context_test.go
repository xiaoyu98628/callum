package service

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/tool"
	toolexecutor "github.com/xiaoyu98628/callum/internal/tool/executor"
	"github.com/xiaoyu98628/callum/internal/tool/policy"
	"github.com/xiaoyu98628/callum/internal/tool/registry"
	"github.com/xiaoyu98628/callum/internal/workspace"
)

func TestExecutionSettingsPromptPreservesMissingHistory(t *testing.T) {
	for _, tc := range []struct {
		name     string
		snapshot *settings.Snapshot
	}{
		{name: "missing"},
		{name: "legacy_mode", snapshot: &settings.Snapshot{Revision: 7, ApprovalPolicy: policy.Always, ApprovalsReviewer: policy.User}},
		{name: "full_access", snapshot: &settings.Snapshot{Revision: 9, ApprovalPolicy: policy.Never, ApprovalsReviewer: policy.User, SandboxMode: tool.SandboxFullAccess}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text, err := executionSettingsPrompt(tc.snapshot)
			if err != nil {
				t.Fatal(err)
			}
			_, raw, found := strings.Cut(text, "\n")
			var value struct {
				Source   string                 `json:"source"`
				Revision *int64                 `json:"settings_revision"`
				Policy   *policy.ApprovalPolicy `json:"approval_policy"`
				Reviewer *policy.Reviewer       `json:"approvals_reviewer"`
				Mode     *tool.SandboxMode      `json:"sandbox_mode"`
			}
			if !found || json.Unmarshal([]byte(raw), &value) != nil || value.Source != "server_execution_settings" {
				t.Fatalf("缺少可信设置投影: %s", text)
			}
			if tc.snapshot == nil {
				if value.Revision != nil || value.Policy != nil || value.Reviewer != nil || value.Mode != nil {
					t.Fatalf("缺失快照被补造: %s", raw)
				}
				return
			}
			if value.Revision == nil || *value.Revision != tc.snapshot.Revision || value.Policy == nil || *value.Policy != tc.snapshot.ApprovalPolicy || value.Reviewer == nil || *value.Reviewer != tc.snapshot.ApprovalsReviewer {
				t.Fatalf("设置投影改变了执行快照: %s", raw)
			}
			if tc.snapshot.SandboxMode == "" {
				if value.Mode != nil {
					t.Fatalf("历史模式被默认值替代: %s", raw)
				}
			} else if value.Mode == nil || *value.Mode != tc.snapshot.SandboxMode {
				t.Fatalf("完全访问被错误描述为受限模式: %s", raw)
			}
		})
	}
	invalid := settings.Snapshot{Revision: 1, ApprovalPolicy: policy.Never, ApprovalsReviewer: policy.AutoReview, SandboxMode: tool.SandboxFullAccess}
	if _, err := executionSettingsPrompt(&invalid); !errors.Is(err, agent.ErrStorage) || !errors.Is(err, settings.ErrInvalid) {
		t.Fatalf("非法快照没有明确失败: %v", err)
	}
}

func TestRequestPartsSeparatesWorkspaceDataFromSettings(t *testing.T) {
	root := filepath.Join(t.TempDir(), "目录\"\n路径文字不是系统指令")
	for _, tc := range []struct {
		name    string
		bound   bool
		project workspace.Workspace
	}{
		{name: "unbound"},
		{name: "without_directory", bound: true},
		{name: "single_directory", bound: true, project: workspace.Workspace{RootPath: root}},
		{name: "multiple_directories", bound: true, project: workspace.Workspace{RootPath: root, OtherPaths: []string{t.TempDir()}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			executor, err := toolexecutor.New(registry.New(), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			engine := &Engine{options: Options{Tools: executor, Workspace: func(_ context.Context, _ string) (workspace.Workspace, error) { return tc.project, nil }}}
			conv := conversation.Conversation{}
			if tc.bound {
				conv.WorkspaceID = new("workspace")
			}
			snapshot := settings.Snapshot{Revision: 3, ApprovalPolicy: policy.OnRequest, ApprovalsReviewer: policy.AutoReview, SandboxMode: tool.SandboxReadOnly}
			facts := []agent.OperationFact{{RequestHash: "unknown-operation"}}
			fixed, definitions, err := engine.requestParts(t.Context(), conv, &snapshot, facts)
			if err != nil {
				t.Fatal(err)
			}
			if len(fixed) != 3 || fixed[0].Role != model.RoleSystem || len(definitions) != 1 || definitions[0].Name != agent.UserInputTool {
				t.Fatalf("固定上下文或工具定义无效: %+v %+v", fixed, definitions)
			}
			if strings.Contains(fixed[0].Content, root) || fixed[1].Role != model.RoleUser || fixed[2].Role != model.RoleUser {
				t.Fatal("路径或历史事实被提升为系统指令")
			}
			if tc.project.RootPath == "" && !strings.Contains(fixed[1].Content, "必须使用绝对路径") {
				t.Fatal("缺少路径基准时未要求绝对路径")
			}
			if tc.bound {
				_, raw, found := strings.Cut(fixed[1].Content, "\n")
				var project struct {
					RootPath   string   `json:"root_path"`
					OtherPaths []string `json:"other_paths"`
				}
				if !found || json.Unmarshal([]byte(raw), &project) != nil || project.RootPath != tc.project.RootPath || len(project.OtherPaths) != len(tc.project.OtherPaths) {
					t.Fatalf("目录数据未按原值编码: %s", fixed[1].Content)
				}
			}
			if !strings.Contains(fixed[2].Content, "unknown-operation") || strings.Count(fixed[0].Content, `"source":"server_execution_settings"`) != 1 {
				t.Fatal("设置重复或丢失待核实操作事实")
			}
		})
	}
}
