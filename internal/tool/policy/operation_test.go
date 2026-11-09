package policy_test

import (
	"testing"

	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/policy"
)

func TestOperationDecisionPrecedenceAndDirectoryGrant(t *testing.T) {
	sandboxed := &tool.CommandBoundary{Sandboxed: true, Mode: tool.SandboxReadOnly}
	for _, tt := range []struct {
		name      string
		policy    policy.ApprovalPolicy
		operation policy.Operation
		want      policy.Decision
		granted   policy.Decision
	}{
		{"rejected_command", policy.OnRequest, policy.Operation{Command: sandboxed, PreviouslyRejected: true, ResultUnconfirmed: true}, policy.Decision{Action: policy.Deny, Reason: "previously_rejected"}, policy.Decision{Action: policy.Deny, Reason: "previously_rejected"}},
		{"unconfirmed_command", policy.OnRequest, policy.Operation{Command: sandboxed, ResultUnconfirmed: true}, policy.Decision{Action: policy.Ask, Reason: "previous_result_requires_confirmation"}, policy.Decision{Action: policy.Ask, Reason: "previous_result_requires_confirmation"}},
		{"unconfirmed_never", policy.Never, policy.Operation{ResultUnconfirmed: true}, policy.Decision{Action: policy.Deny, Reason: "previous_result_requires_confirmation"}, policy.Decision{Action: policy.Deny, Reason: "previous_result_requires_confirmation"}},
		{"unconfirmed_always", policy.Always, policy.Operation{ResultUnconfirmed: true}, policy.Decision{Action: policy.Ask, Reason: "previous_result_requires_confirmation"}, policy.Decision{Action: policy.Ask, Reason: "previous_result_requires_confirmation"}},
		{"read_only_write", policy.OnRequest, policy.Operation{Effects: []tool.Effect{tool.EffectWrite}, SandboxMode: tool.SandboxReadOnly, OutsideWorkspace: true}, policy.Decision{Action: policy.Ask, Reason: "read_only_mode"}, policy.Decision{Action: policy.Ask, Reason: "read_only_mode"}},
		{"outside_write", policy.OnRequest, policy.Operation{Effects: []tool.Effect{tool.EffectWrite}, OutsideWorkspace: true}, policy.Decision{Action: policy.Ask, Reason: "outside_workspace"}, policy.Decision{Action: policy.Allow, Reason: "conversation_directory_grant"}},
		{"always_write", policy.Always, policy.Operation{Effects: []tool.Effect{tool.EffectWrite}, OutsideWorkspace: true}, policy.Decision{Action: policy.Ask, Reason: "policy_always"}, policy.Decision{Action: policy.Ask, Reason: "policy_always"}},
		{"never_write", policy.Never, policy.Operation{Effects: []tool.Effect{tool.EffectWrite}, OutsideWorkspace: true}, policy.Decision{Action: policy.Deny, Reason: "approval_required_but_disabled"}, policy.Decision{Action: policy.Deny, Reason: "approval_required_but_disabled"}},
		{"sandboxed_command", policy.OnRequest, policy.Operation{Effects: []tool.Effect{tool.EffectUnknown}, Command: sandboxed}, policy.Decision{Action: policy.Allow, Reason: "sandboxed_command"}, policy.Decision{Action: policy.Allow, Reason: "sandboxed_command"}},
		{"escalated_command", policy.OnRequest, policy.Operation{Command: &tool.CommandBoundary{Escalated: true}}, policy.Decision{Action: policy.Ask, Reason: "sandbox_escalation"}, policy.Decision{Action: policy.Ask, Reason: "sandbox_escalation"}},
		{"unknown_effect", policy.OnRequest, policy.Operation{Effects: []tool.Effect{tool.EffectUnknown}, OutsideWorkspace: true}, policy.Decision{Action: policy.Ask, Reason: "unclassified_operation"}, policy.Decision{Action: policy.Ask, Reason: "unclassified_operation"}},
		{"invalid_policy", "invalid", policy.Operation{Command: sandboxed}, policy.Decision{Action: policy.Deny, Reason: "invalid_approval_policy"}, policy.Decision{Action: policy.Deny, Reason: "invalid_approval_policy"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			decision := policy.EvaluateOperation(tt.policy, tt.operation)
			if decision != tt.want {
				t.Fatalf("判定错误: got=%+v want=%+v", decision, tt.want)
			}
			if actual := policy.ApplyDirectoryGrant(decision, false); actual != decision {
				t.Fatalf("未匹配授权却改变判定: %+v", actual)
			}
			if actual := policy.ApplyDirectoryGrant(decision, true); actual != tt.granted {
				t.Fatalf("授权覆盖了不应解除的约束: got=%+v want=%+v", actual, tt.granted)
			}
		})
	}
}
