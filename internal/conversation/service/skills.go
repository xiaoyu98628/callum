package service

import (
	"context"
	"fmt"
	"slices"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/skills"
)

// 受理只核对已安装目录，客户端路径不能使任意文件成为技能入口。
func (s *Service) validateSkills(ctx context.Context, content []model.Block) error {
	index := slices.IndexFunc(content, func(block model.Block) bool { return block.Type == "skill" })
	if index < 0 {
		return nil
	}
	entries, err := s.skills.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("%w: 读取技能目录: %w", agent.ErrStorage, err)
	}
	selected := content[index]
	if !slices.ContainsFunc(entries, func(entry skills.Entry) bool { return entry.Name == selected.Name && entry.Path == selected.Path }) {
		return conversation.ErrInvalid
	}
	return nil
}
