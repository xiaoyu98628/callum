package policy

import (
	"path/filepath"
	"slices"

	"github.com/xiaoyu98628/callum/internal/tool"
)

// DirectoryCovers 只接受全部已分类为读取或普通修改的固定文件目标，检查辅助文件路径。
func DirectoryCovers(s tool.DirectoryScope, target tool.Target, effects []tool.Effect) bool {
	if !s.Valid() || !filepath.IsAbs(target.Path) || len(effects) == 0 || (target.Kind != "file" && target.Kind != "directory") {
		return false
	}
	for _, effect := range effects {
		if effect != tool.EffectRead && (effect != tool.EffectWrite || s.Access != "read_write") {
			return false
		}
	}
	for _, path := range []string{target.Path, target.RecoveryPath, target.StagingPath} {
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			return false
		}
		rel, err := filepath.Rel(s.RootPath, path)
		if err != nil || !filepath.IsLocal(rel) {
			return false
		}
	}
	return true
}

// DirectoriesCover 要求工作区外的所有固定目标及辅助路径都被授权覆盖。
// 工作区内目标已由 on-request 策略授权；无文件目标的命令不能继承目录权限。
func DirectoriesCover(scopes []tool.DirectoryScope, targets []tool.Target, effects []tool.Effect) bool {
	outside := false
	for _, target := range targets {
		if target.WithinWorkspace {
			continue
		}
		outside = true
		if !slices.ContainsFunc(scopes, func(scope tool.DirectoryScope) bool { return DirectoryCovers(scope, target, effects) }) {
			return false
		}
	}
	return outside
}

// DirectoryOption 从已固定并参与操作指纹的目标推导授权选项，不改变历史操作身份。
// 创建目录的计划必须包含所有缺失父目录；由它们共同确定现有父目录范围。
// 不为跨盘或隐式扩大到磁盘根目录的操作提供目录授权选项。
func DirectoryOption(targets []tool.Target, effects []tool.Effect) *tool.DirectoryScope {
	if len(targets) == 0 || len(effects) == 0 {
		return nil
	}
	access := "read"
	for _, effect := range effects {
		if effect != tool.EffectRead && effect != tool.EffectWrite {
			return nil
		}
		if effect == tool.EffectWrite {
			access = "read_write"
		}
	}
	var root string
	for _, target := range targets {
		if !filepath.IsAbs(target.Path) || (target.Kind != "file" && target.Kind != "directory") {
			return nil
		}
		dir := target.Path
		if target.Kind != "directory" || target.Action == "create" {
			dir = filepath.Dir(dir)
		}
		if root == "" {
			root = dir
		}
		for {
			rel, err := filepath.Rel(root, dir)
			if err != nil {
				return nil
			}
			if filepath.IsLocal(rel) {
				break
			}
			next := filepath.Dir(root)
			if next == root {
				return nil
			}
			root = next
		}
	}
	if filepath.Dir(root) == root && !(len(targets) == 1 && targets[0].Kind == "directory" && targets[0].Path == root) {
		return nil
	}
	scope := tool.DirectoryScope{RootPath: root, Access: access}
	if slices.ContainsFunc(targets, func(target tool.Target) bool { return !DirectoryCovers(scope, target, effects) }) {
		return nil
	}
	return &scope
}
