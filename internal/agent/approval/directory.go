package approval

import (
	"context"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/policy"
)

// ListDirectories 按会话分页读取授权及其撤销记录，权限读取失败不回退为放行。
func (s *Service) ListDirectories(ctx context.Context, conversationID string, limit, offset int) ([]agent.DirectoryGrant, error) {
	if limit == 0 {
		limit = 100
	}
	if !validID(conversationID) || limit < 1 || limit > 200 || offset < 0 {
		return nil, agent.ErrApprovalInvalid
	}
	return s.repository.ListDirectoryGrants(ctx, conversationID, limit, offset)
}

// RevokeDirectory 保存撤销后发布业务事件；来源审批与已认领的操作保持原义。
func (s *Service) RevokeDirectory(ctx context.Context, conversationID, id string) (agent.DirectoryGrant, error) {
	if !validID(conversationID) || !validID(id) {
		return agent.DirectoryGrant{}, agent.ErrApprovalInvalid
	}
	value, err := s.repository.RevokeDirectoryGrant(ctx, conversationID, id)
	if err != nil {
		return value, err
	}
	return value, s.emit(agent.Event{Type: "directory_grant.updated", ConversationID: conversationID, Data: value})
}

// MatchDirectories 每次查询当前会话授权，不缓存撤销状态；认领工具时由仓储在事务内再次核验。
func (s *Service) MatchDirectories(ctx context.Context, conversationID string, targets []tool.Target, effects []tool.Effect) ([]string, error) {
	covered := make([]bool, len(targets))
	remaining := 0
	for i, target := range targets {
		covered[i] = target.WithinWorkspace
		if !covered[i] {
			remaining++
		}
	}
	if remaining == 0 {
		return nil, nil
	}
	var ids []string
	used := map[string]bool{}
	for offset := 0; ; offset += 200 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := s.ListDirectories(ctx, conversationID, 200, offset)
		if err != nil {
			return nil, err
		}
		for _, grant := range page {
			if grant.RevokedAt != nil {
				continue
			}
			for i, target := range targets {
				if !covered[i] && policy.DirectoryCovers(grant.Scope, target, effects) {
					covered[i] = true
					remaining--
					if !used[grant.ID] {
						ids = append(ids, grant.ID)
						used[grant.ID] = true
					}
				}
			}
		}
		if remaining == 0 {
			return ids, nil
		}
		if len(page) < 200 {
			break
		}
	}
	return nil, nil
}
