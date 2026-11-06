package settings_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/tool"
)

func TestExecutionSettingsRejectAmbiguousOrUnsupportedValues(t *testing.T) {
	for _, raw := range []string{
		`null`, `[]`, `{}`, `{"approval_policy":"on-request"}`,
		`{"approval_policy":null,"approvals_reviewer":"user"}`,
		`{"approval_policy":"never","approvals_reviewer":"auto_review"}`,
		`{"approval_policy":"on-request","approvals_reviewer":"user","sandbox_mode":"disabled"}`,
		`{"approval_policy":"on-request","approvals_reviewer":"user","sandbox_mode":null}`,
		`{"approval_policy":"on-request","approvals_reviewer":"user","sandbox_mode":""}`,
		`{"approval_policy":"never","approval_policy":"on-request","approvals_reviewer":"user"}`,
		`{"approval_policy":"never","approvals_reviewer":"user"} {}`,
		string([]byte{'{', '"', 0xff, '"', ':', '1', '}'}),
		strings.Repeat(" ", settings.MaxValueBytes+1),
	} {
		if _, err := settings.DecodeExecution(json.RawMessage(raw)); !errors.Is(err, settings.ErrInvalid) {
			t.Fatalf("接受了非法配置: %v", err)
		}
	}
	for _, choice := range []string{"on-request", "always", "never"} {
		value, err := settings.DecodeExecution(json.RawMessage(`{"approval_policy":"` + choice + `","approvals_reviewer":"user"}`))
		if err != nil || string(value.ApprovalPolicy) != choice {
			t.Fatalf("拒绝了有效配置: %+v %v", value, err)
		}
	}
}

func TestSandboxDefaultDoesNotRewriteHistoricalSnapshots(t *testing.T) {
	raw := json.RawMessage(`{"approval_policy":"on-request","approvals_reviewer":"user"}`)
	current, err := settings.DecodeExecution(raw)
	if err != nil || current.SandboxMode != tool.SandboxWorkspaceWrite {
		t.Fatalf("旧全局配置的新任务默认值错误: %+v %v", current, err)
	}
	var old settings.Snapshot
	if err := json.Unmarshal([]byte(`{"revision":1,"approval_policy":"on-request","approvals_reviewer":"user"}`), &old); err != nil || !old.Valid() || old.SandboxMode != "" {
		t.Fatalf("历史快照被扩大权限: %+v %v", old, err)
	}
}
