package tool_test

import (
	"path/filepath"
	"testing"

	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/policy"
)

func TestDirectoryScopeBoundaries(t *testing.T) {
	root := t.TempDir()
	read := tool.DirectoryScope{RootPath: root, Access: "read"}
	write := tool.DirectoryScope{RootPath: root, Access: "read_write"}
	for _, test := range []struct {
		name    string
		scope   tool.DirectoryScope
		target  tool.Target
		effects []tool.Effect
		want    bool
	}{
		{"descendant", read, tool.Target{Path: filepath.Join(root, "a", "b.txt"), Kind: "file"}, []tool.Effect{tool.EffectRead}, true},
		{"same_directory", read, tool.Target{Path: root, Kind: "directory"}, []tool.Effect{tool.EffectRead}, true},
		{"prefix_sibling", read, tool.Target{Path: root + "-other", Kind: "directory"}, []tool.Effect{tool.EffectRead}, false},
		{"parent", read, tool.Target{Path: filepath.Dir(root), Kind: "directory"}, []tool.Effect{tool.EffectRead}, false},
		{"read_does_not_write", read, tool.Target{Path: filepath.Join(root, "a"), Kind: "file"}, []tool.Effect{tool.EffectWrite}, false},
		{"write_includes_read", write, tool.Target{Path: filepath.Join(root, "a"), Kind: "file"}, []tool.Effect{tool.EffectRead}, true},
		{"ordinary_write", write, tool.Target{Path: filepath.Join(root, "a"), Kind: "file"}, []tool.Effect{tool.EffectWrite}, true},
		{"destructive", write, tool.Target{Path: filepath.Join(root, "a"), Kind: "file"}, []tool.Effect{tool.EffectRead, tool.EffectDestructive}, false},
		{"unknown", write, tool.Target{Path: filepath.Join(root, "a"), Kind: "file"}, []tool.Effect{tool.EffectUnknown}, false},
		{"external", write, tool.Target{Path: filepath.Join(root, "a"), Kind: "file"}, []tool.Effect{tool.EffectExternal}, false},
		{"unclassified", write, tool.Target{Path: filepath.Join(root, "a"), Kind: "file"}, nil, false},
		{"auxiliary_outside", write, tool.Target{Path: filepath.Join(root, "a"), Kind: "file", RecoveryPath: filepath.Join(filepath.Dir(root), "backup")}, []tool.Effect{tool.EffectWrite}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := policy.DirectoryCovers(test.scope, test.target, test.effects); got != test.want {
				t.Fatalf("目录范围错误: got=%v want=%v", got, test.want)
			}
		})
	}
	targets := []tool.Target{{Path: filepath.Join(root, "a"), Kind: "file"}, {Path: root + "-other", Kind: "file"}}
	if policy.DirectoriesCover([]tool.DirectoryScope{read}, targets, []tool.Effect{tool.EffectRead}) {
		t.Fatal("多目标操作只覆盖一部分即被放行")
	}
	if policy.DirectoriesCover([]tool.DirectoryScope{write}, nil, []tool.Effect{tool.EffectRead}) {
		t.Fatal("无文件目标操作继承了目录权限")
	}
}
