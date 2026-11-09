package policy_test

import (
	"testing"

	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/policy"
)

func TestApprovalPolicyDoesNotExpandOperationPermissions(t *testing.T) {
	for _, test := range []struct {
		name    string
		effects []tool.Effect
		request policy.Action
		never   policy.Action
	}{
		{"read", []tool.Effect{tool.EffectRead}, policy.Allow, policy.Allow},
		{"ordinary_write", []tool.Effect{tool.EffectWrite}, policy.Allow, policy.Allow},
		{"read_write", []tool.Effect{tool.EffectRead, tool.EffectWrite}, policy.Allow, policy.Allow},
		{"delete_or_overwrite", []tool.Effect{tool.EffectDestructive}, policy.Ask, policy.Deny},
		{"external", []tool.Effect{tool.EffectExternal}, policy.Ask, policy.Deny},
		{"unknown", []tool.Effect{tool.EffectUnknown}, policy.Ask, policy.Deny},
		{"missing", nil, policy.Ask, policy.Deny},
		{"read_and_delete", []tool.Effect{tool.EffectRead, tool.EffectDestructive}, policy.Ask, policy.Deny},
		{"write_and_external", []tool.Effect{tool.EffectWrite, tool.EffectExternal}, policy.Ask, policy.Deny},
		{"read_and_unknown", []tool.Effect{tool.EffectRead, tool.EffectUnknown}, policy.Ask, policy.Deny},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, choice := range []struct {
				policy policy.ApprovalPolicy
				want   policy.Action
			}{{policy.OnRequest, test.request}, {policy.Always, policy.Ask}, {policy.Never, test.never}} {
				got := policy.Evaluate(choice.policy, test.effects, false)
				if got.Action != choice.want || got.Reason == "" {
					t.Fatalf("%s: %+v，期望 %s", choice.policy, got, choice.want)
				}
			}
		})
	}
}

func TestCommandBoundaryIsIndependentOfApprovalPolicy(t *testing.T) {
	for _, test := range []struct {
		name             string
		boundary         tool.CommandBoundary
		onRequest, never policy.Action
	}{
		{"workspace", tool.CommandBoundary{Sandboxed: true, Mode: tool.SandboxWorkspaceWrite}, policy.Ask, policy.Deny},
		{"readonly", tool.CommandBoundary{Sandboxed: true, Mode: tool.SandboxReadOnly}, policy.Allow, policy.Allow},
		{"elevated", tool.CommandBoundary{Escalated: true, Mode: tool.SandboxWorkspaceWrite}, policy.Ask, policy.Deny},
		{"destructive", tool.CommandBoundary{Sandboxed: true, Mode: tool.SandboxWorkspaceWrite, Risk: "destructive_command"}, policy.Ask, policy.Deny},
		{"legacy", tool.CommandBoundary{}, policy.Ask, policy.Deny},
		{"unsupported", tool.CommandBoundary{Mode: tool.SandboxWorkspaceWrite, Risk: "sandbox_backend_unsupported"}, policy.Ask, policy.Deny},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, choice := range []struct {
				p    policy.ApprovalPolicy
				want policy.Action
			}{{policy.OnRequest, test.onRequest}, {policy.Never, test.never}, {policy.Always, policy.Ask}} {
				if got := policy.EvaluateCommand(choice.p, test.boundary); got.Action != choice.want || got.Reason == "" {
					t.Fatalf("审批扩大了边界: %s %+v", choice.p, got)
				}
			}
		})
	}
}

func TestInvalidPolicyAndOperationFailClosed(t *testing.T) {
	for _, p := range []policy.ApprovalPolicy{"", "auto-read", "allow-all"} {
		if got := policy.Evaluate(p, []tool.Effect{tool.EffectRead}, false); got.Action != policy.Deny {
			t.Fatalf("未知审批策略被放行: %s %+v", p, got)
		}
	}
	for _, p := range []policy.ApprovalPolicy{policy.OnRequest, policy.Always, policy.Never} {
		if got := policy.Evaluate(p, []tool.Effect{tool.EffectRead, "invalid"}, false); got.Action != policy.Deny {
			t.Fatalf("无效类别被忽略或允许审批: %s %+v", p, got)
		}
	}
}

func TestOutsideWorkspaceWritesRequireApproval(t *testing.T) {
	for _, test := range []struct {
		policy policy.ApprovalPolicy
		want   policy.Action
	}{
		{policy.OnRequest, policy.Ask}, {policy.Always, policy.Ask}, {policy.Never, policy.Deny},
	} {
		for _, effects := range [][]tool.Effect{{tool.EffectWrite}, {tool.EffectRead, tool.EffectWrite}, {tool.EffectRead, tool.EffectDestructive}, {tool.EffectRead, tool.EffectExternal}, {tool.EffectRead, tool.EffectUnknown}, nil} {
			if got := policy.Evaluate(test.policy, effects, true); got.Action != test.want {
				t.Fatalf("工作区外访问被错误放行: %s %+v", test.policy, got)
			}
		}
	}
}

func TestReadOnlyAccessIndependentOfWorkspace(t *testing.T) {
	for _, outside := range []bool{false, true} {
		for _, choice := range []struct {
			policy policy.ApprovalPolicy
			want   policy.Action
		}{
			{policy.OnRequest, policy.Allow}, {policy.Never, policy.Allow}, {policy.Always, policy.Ask},
		} {
			got := policy.Evaluate(choice.policy, []tool.Effect{tool.EffectRead}, outside)
			if got.Action != choice.want {
				t.Fatalf("纯读取策略错误: outside=%v policy=%s got=%+v", outside, choice.policy, got)
			}
			if choice.want == policy.Allow && got.Reason != "read_only_operation" {
				t.Fatalf("只读来源错误: %+v", got)
			}
		}
	}
}
