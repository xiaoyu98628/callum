package filesystem

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/tool"
)

func TestSkillReadExceptionDoesNotExposeApplicationData(t *testing.T) {
	data := t.TempDir()
	data, err := filepath.EvalSymlinks(data)
	if err != nil {
		t.Fatal(err)
	}
	skills := filepath.Join(data, "skills")
	if err := os.Mkdir(skills, 0700); err != nil {
		t.Fatal(err)
	}
	database := filepath.Join(data, "callum.db")
	if err := os.WriteFile(database, []byte("private database"), 0600); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(skills, "SKILL.md")
	if err := os.WriteFile(entry, []byte("skill-body"), 0600); err != nil {
		t.Fatal(err)
	}
	protection := tool.ProtectedData{Directory: data, Database: database, ReadOnlyDirectories: []string{skills}}
	var reader tool.Tool
	for _, instance := range Tools(protection) {
		if instance.Definition().Name == "read_file" {
			reader = instance
		}
	}
	read := func(path string) (tool.Result, error) {
		raw, err := json.Marshal(map[string]any{"path": path})
		if err != nil {
			return tool.Result{}, err
		}
		call, err := reader.Prepare(t.Context(), tool.Scope{}, raw)
		if err != nil {
			return tool.Result{}, err
		}
		result, executeErr := call.Execute(t.Context())
		return result, errors.Join(executeErr, call.Close())
	}
	result, err := read(entry)
	if err != nil || !strings.Contains(string(result.Output), "skill-body") {
		t.Fatalf("技能不可读: %s %v", result.Output, err)
	}
	if _, err := read(database); !errors.Is(err, tool.ErrDenied) {
		t.Fatalf("数据库可直接读取: %v", err)
	}
	t.Run("symlink", func(t *testing.T) {
		path := filepath.Join(skills, "escape.md")
		if err := os.Symlink(database, path); err != nil {
			t.Skipf("平台无法创建符号链接: %v", err)
		}
		if _, err := read(path); !errors.Is(err, tool.ErrDenied) {
			t.Fatalf("链接绕过私有数据边界: %v", err)
		}
	})
	t.Run("hardlink", func(t *testing.T) {
		path := filepath.Join(skills, "alias.md")
		if err := os.Link(database, path); err != nil {
			t.Skipf("平台无法创建硬链接: %v", err)
		}
		if _, err := read(path); !errors.Is(err, tool.ErrDenied) {
			t.Fatalf("数据库身份校验失效: %v", err)
		}
	})
	if !protection.WriteProtected(entry) {
		t.Fatal("技能目录意外开放写入")
	}
}
