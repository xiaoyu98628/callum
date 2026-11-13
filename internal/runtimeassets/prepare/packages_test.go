package prepare

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func wheelFixture(t *testing.T, directory, name string, files map[string]string) string {
	t.Helper()
	path := filepath.Join(directory, name+".whl")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestWheelCompositionPreservesModulesAndRejectsOverwrite(t *testing.T) {
	base := t.TempDir()
	module := "module/__init__.py"
	first := wheelFixture(t, base, "first", map[string]string{module: "first"})
	second := wheelFixture(t, base, "second", map[string]string{module: "replaced"})
	target := Target{Platform: "darwin", Python: Source{Version: "3.14.10"}}
	for _, name := range []string{"first", "second"} {
		target.PythonPackages = append(target.PythonPackages, PackageSource{Name: name, Version: "1", URL: "https://files.pythonhosted.org/" + name + ".whl", SHA256: strings.Repeat("0", 64), Archive: name + ".whl", Prefix: ".", Entry: module})
	}
	stage := t.TempDir()
	if err := installPackages(t.Context(), target, stage, []string{first, second}); err == nil {
		t.Fatal("后一个 wheel 覆盖了已安装模块")
	}
	data, err := os.ReadFile(filepath.Join(stage, packageDirectory(target), filepath.FromSlash(module)))
	if err != nil || string(data) != "first" {
		t.Fatalf("已有模块被修改: %q %v", data, err)
	}
	if err := extractInto(t.Context(), wheelFixture(t, base, "unsafe", map[string]string{"../escape": "no"}), t.TempDir(), "."); err == nil {
		t.Fatal("wheel 路径越界未拒绝")
	}
	target.PythonPackages[0].URL = "https://example.org/untrusted.whl"
	if err := validatePackages(target); err == nil {
		t.Fatal("未拒绝非固定归档来源")
	}
}
