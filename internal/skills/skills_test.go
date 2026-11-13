package skills

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	value, err := Open(t.Context(), t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func writeSkill(t *testing.T, store *Store, directory, content string) string {
	t.Helper()
	path := filepath.Join(store.Directory(), directory, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDiscoveryRefreshesUserSkillsAndPreservesSources(t *testing.T) {
	store := newStore(t)
	if values, err := store.Snapshot(t.Context()); err != nil || len(values) != 0 {
		t.Fatalf("空目录: %v %v", values, err)
	}
	path := writeSkill(t, store, "custom", "---\nname: \"Custom Skill\"\ndescription: >-\n  Analyze logs\n  when requested.\nmetadata:\n  short-description: logs\n---\nprivate-body-marker")
	values, err := store.Snapshot(t.Context())
	if err != nil || len(values) != 1 || values[0].Description != "Analyze logs when requested." || values[0].Path != path {
		t.Fatalf("未识别通用技能: %+v %v", values, err)
	}
	writeSkill(t, store, "nested/another", "---\nname: Custom Skill\ndescription: another source\n---\n")
	if values, err := store.Snapshot(t.Context()); err != nil || len(values) != 2 {
		t.Fatalf("同名技能被覆盖: %+v %v", values, err)
	}
	writeSkill(t, store, "custom", "---\nname: Custom Skill\ndescription: updated description\n---\nprivate-body-marker")
	if values, err := store.Snapshot(t.Context()); err != nil || values[0].Description != "updated description" {
		t.Fatalf("技能元数据修改未刷新: %+v %v", values, err)
	}
	if err := os.MkdirAll(filepath.Join(store.Directory(), "custom", "agents"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Directory(), "custom", "agents", "callum.yaml"), []byte("policy:\n  allow_implicit_invocation: false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	contextText, err := store.Context(t.Context())
	if err != nil || strings.Contains(contextText, "private-body-marker") || !strings.Contains(contextText, `"explicit_only":true`) {
		t.Fatalf("没有按需加载或策略丢失: %s %v", contextText, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if values, err := store.Snapshot(t.Context()); err != nil || len(values) != 1 {
		t.Fatalf("删除未生效: %+v %v", values, err)
	}
}

func TestInvalidSkillsDoNotHideValidSkills(t *testing.T) {
	store := newStore(t)
	writeSkill(t, store, "good", "---\nname: good\ndescription: valid\n---\n")
	for name, content := range map[string]string{
		"no-header": "plain markdown",
		"duplicate": "---\nname: first\nname: duplicate\ndescription: duplicate key\n---\n",
		"missing":   "---\nname: missing\n---\n",
		"large":     strings.Repeat("x", maxDocumentBytes+1),
	} {
		writeSkill(t, store, name, content)
	}
	if runtime.GOOS != "windows" {
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(store.Directory(), "linked")); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(outside, "SKILL.md"), []byte("---\nname: outside\ndescription: no\n---"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	values, err := store.Snapshot(t.Context())
	if err != nil || len(values) != 1 || values[0].Name != "good" {
		t.Fatalf("无效技能影响其他技能: %+v %v", values, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.Snapshot(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消未传递: %v", err)
	}
}

func TestSystemSkillsAndUserOverridesRefresh(t *testing.T) {
	store := newStore(t)
	system := writeSkill(t, store, ".system/documents", "---\nname: documents\ndescription: bundled\n---\n")
	writeSkill(t, store, ".hidden/ignored", "---\nname: hidden\ndescription: ignored\n---\n")
	values, err := store.Snapshot(t.Context())
	if err != nil || len(values) != 1 || values[0].Path != system || values[0].Source != "system" {
		t.Fatalf("系统技能未被发现: %+v %v", values, err)
	}
	custom := writeSkill(t, store, "my-documents", "---\nname: documents\ndescription: customized\n---\n")
	values, err = store.Snapshot(t.Context())
	if err != nil || len(values) != 1 || values[0].Path != custom || values[0].Source != "user" {
		t.Fatalf("用户覆盖未生效: %+v %v", values, err)
	}
	if err := os.Remove(custom); err != nil {
		t.Fatal(err)
	}
	values, err = store.Snapshot(t.Context())
	if err != nil || len(values) != 1 || values[0].Path != system {
		t.Fatalf("删除覆盖后未恢复系统技能: %+v %v", values, err)
	}
}
