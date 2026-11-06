package agent_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/tool"
)

func TestDirectoryRevocationBeforeAtomicClaim(t *testing.T) {
	_, r, ex, step := fixture(t)
	root := t.TempDir()
	scope := tool.DirectoryScope{RootPath: root, Access: "read"}
	request, err := json.Marshal(map[string]any{"tool": tool.Definition{Effects: []tool.Effect{tool.EffectRead}}, "target": tool.Target{Path: root, Kind: "directory"}, "directory_scope": scope})
	if err != nil {
		t.Fatal(err)
	}
	step.Input, err = json.Marshal(map[string]any{"approval_policy": "on-request", "authorization": map[string]any{"action": "ask", "reason": "outside_workspace"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.SaveStep(t.Context(), step); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(request)
	a, err := r.CreateApproval(t.Context(), agent.Approval{ID: newID(t), RunID: step.RunID, StepID: step.ID, Request: request, RequestHash: hex.EncodeToString(sum[:]), CreatedAt: 1, UpdatedAt: 1})
	if err != nil {
		t.Fatal(err)
	}
	a, err = r.DecideApproval(t.Context(), a.ID, agent.ApprovalApproved, new("user"), nil, agent.ApprovalConversationDirectory)
	if err != nil || a.DirectoryGrant == nil {
		t.Fatalf("未创建授权: %+v %v", a, err)
	}
	if _, err := r.DecideApproval(t.Context(), a.ID, agent.ApprovalApproved, new("user"), nil, agent.ApprovalConversationDirectory); err != nil {
		t.Fatal(err)
	}
	if _, err := r.StartTool(t.Context(), step.RunID, step.ID, a.ID, a.RequestHash); err != nil {
		t.Fatal(err)
	}
	inherited := step
	inherited.ID = newID(t)
	inherited.Status = agent.Pending
	inherited.Sequence = 0
	inherited.Input, err = json.Marshal(map[string]any{"approval_policy": "on-request", "authorization": map[string]any{"action": "allow", "reason": "conversation_directory_grant"}, "directory_grant_ids": []string{a.DirectoryGrant.ID}, "request": map[string]any{"tool": tool.Definition{Effects: []tool.Effect{tool.EffectRead}}, "target": tool.Target{Path: filepath.Join(root, "child.txt"), Kind: "file"}}})
	if err != nil {
		t.Fatal(err)
	}
	inherited, err = r.CreateStep(t.Context(), inherited)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RevokeDirectoryGrant(t.Context(), ex.Conversation.ID, a.DirectoryGrant.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.StartTool(t.Context(), step.RunID, inherited.ID, "", "unused"); !errors.Is(err, tool.ErrDenied) {
		t.Fatalf("撤销后仍可认领工具: %v", err)
	}
	if _, err := r.DecideApproval(t.Context(), a.ID, agent.ApprovalApproved, new("user"), nil, agent.ApprovalConversationDirectory); err != nil {
		t.Fatal(err)
	}
	grants, err := r.ListDirectoryGrants(t.Context(), ex.Conversation.ID, 100, 0)
	if err != nil || len(grants) != 1 || grants[0].RevokedAt == nil {
		t.Fatalf("幂等提交重新激活了撤销授权: %+v %v", grants, err)
	}
}
