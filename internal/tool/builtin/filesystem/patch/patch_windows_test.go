package patch

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/xiaoyu98628/callum/internal/tool"
)

func TestPatchPreservesWindowsDACLAndProtectsNewFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note")
	before := "header\r\nold\r\ntail\r\n"
	patchTestWrite(t, path, before)
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := protectPatchFile(f); err != nil {
		f.Close()
		t.Fatal(err)
	}
	sd, err := windows.GetSecurityInfo(windows.Handle(f.Fd()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		f.Close()
		t.Fatal(err)
	}
	original := sd.String()
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	e := patchTestExecutor(t)
	p := patchTestPrepare(t, e, tool.Scope{ID: "w", RootPath: dir}, patchTestArgs(t, "*** Update File: note\n@@\n header\n-old\n+new\n tail\n*** Add File: nested/new.txt\n+created", patchVersion{"note", textSHA256([]byte(before))}))
	if result, err := e.Execute(t.Context(), p, p.Hash()); err != nil || result.Outcome != tool.OutcomeApplied {
		t.Fatalf("修改失败: %s %v", result.Output, err)
	}
	for _, target := range []string{path, filepath.Join(dir, "nested", "new.txt")} {
		file, err := os.Open(target)
		if err != nil {
			t.Fatal(err)
		}
		sd, securityErr := windows.GetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		closeErr := file.Close()
		if securityErr != nil || closeErr != nil {
			t.Fatalf("读取权限失败: %v %v", securityErr, closeErr)
		}
		if sd.String() != original {
			t.Fatalf("DACL 未保留或新文件权限不私有: %s != %s", sd.String(), original)
		}
	}
}
