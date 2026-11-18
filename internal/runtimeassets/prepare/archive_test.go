package prepare

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func archiveFixture(t *testing.T, archive string, headers ...*tar.Header) {
	t.Helper()
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	compressed := gzip.NewWriter(file)
	writer := tar.NewWriter(compressed)
	for _, header := range headers {
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeReg {
			if _, err := writer.Write(make([]byte, header.Size)); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, close := range []func() error{writer.Close, compressed.Close, file.Close} {
		if err := close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOfflinePreparationRejectsMissingArchivesWithoutDownloading(t *testing.T) {
	cache, output := t.TempDir(), t.TempDir()
	source := Source{Version: "fixed", URL: "https://example.invalid/must-not-download", SHA256: strings.Repeat("0", 64), Archive: "missing.tar.gz", Prefix: "root", Entry: "bin/interpreter"}
	target := Target{Directory: "target", Platform: runtime.GOOS, Arch: runtime.GOARCH, Python: source, Node: source}
	if err := Install(t.Context(), target, output, cache, true); err == nil || !strings.Contains(err.Error(), "离线模式不允许下载") {
		t.Fatalf("离线准备没有直接报告缺失的本地归档: %v", err)
	}
	entries, err := os.ReadDir(output)
	if err != nil || len(entries) != 0 {
		t.Fatalf("离线失败遗留了资源或锁: %v %v", entries, err)
	}
}

func TestArchiveRejectsTraversalLinksAndSpecialFiles(t *testing.T) {
	for _, test := range []struct {
		name   string
		header *tar.Header
	}{
		{"traversal", &tar.Header{Name: "root/../escape", Typeflag: tar.TypeReg, Mode: 0644}},
		{"absolute", &tar.Header{Name: "/outside", Typeflag: tar.TypeReg, Mode: 0644}},
		{"symlink", &tar.Header{Name: "root/bin/python", Typeflag: tar.TypeSymlink, Linkname: "../../outside"}},
		{"hardlink", &tar.Header{Name: "root/alias", Typeflag: tar.TypeLink, Linkname: "root/bin/python"}},
		{"device", &tar.Header{Name: "root/device", Typeflag: tar.TypeChar, Mode: 0644}},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := t.TempDir()
			archive := filepath.Join(base, "archive.tar.gz")
			archiveFixture(t, archive, test.header)
			if err := extract(t.Context(), archive, filepath.Join(base, "payload"), "root"); err == nil {
				t.Fatal("接受了不安全的归档")
			}
			if _, err := os.Stat(filepath.Join(base, "escape")); !os.IsNotExist(err) {
				t.Fatal("归档写出了暂存目录")
			}
		})
	}
}

func TestInstallChecksArchiveAndNeverOverwritesExistingResources(t *testing.T) {
	cache, output := t.TempDir(), t.TempDir()
	makeSource := func(name, entry string) Source {
		archive := filepath.Join(cache, name+".tar.gz")
		archiveFixture(t, archive, &tar.Header{Name: "root/" + entry, Typeflag: tar.TypeReg, Mode: 0755, Size: 16}, &tar.Header{Name: "root/libexec/helper", Typeflag: tar.TypeReg, Mode: 0755}, &tar.Header{Name: "root/templates/sample", Typeflag: tar.TypeReg, Mode: 0644}, &tar.Header{Name: "root/etc/gitconfig", Typeflag: tar.TypeReg, Mode: 0644})
		data, err := os.ReadFile(archive)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		return Source{Version: "fixed", URL: "https://example.invalid/archive", SHA256: hex.EncodeToString(sum[:]), Archive: name + ".tar.gz", Prefix: "root", Entry: entry}
	}
	target := Target{Directory: "target", Platform: runtime.GOOS, Arch: runtime.GOARCH, Python: makeSource("python", "bin/python3"), Node: makeSource("node", "bin/node"), Git: GitSource{Source: makeSource("git", "bin/git"), ExecPath: "libexec", TemplatePath: "templates", ConfigPath: "etc/gitconfig"}}
	if err := Install(t.Context(), target, output, cache, true); err != nil {
		t.Fatal(err)
	}
	// 临时资源解除目录只读权限后交给 t.TempDir 回收，不触碰应用或用户目录。
	t.Cleanup(func() {
		if err := removeStage(filepath.Join(output, "target")); err != nil {
			t.Error(err)
		}
	})
	if err := Install(t.Context(), target, output, cache, true); err != nil {
		t.Fatal("同一资源准备不幂等:", err)
	}
	if err := os.Chmod(filepath.Join(output, "target", "python", "bin", "python3"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "target", "python", "bin", "python3"), []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Install(t.Context(), target, output, cache, true); err == nil {
		t.Fatal("覆盖了已有的损坏资源")
	}
	target.Directory = "checksum-failure"
	target.Python.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	if err := Install(t.Context(), target, output, cache, true); err == nil {
		t.Fatal("未校验归档内容")
	}
	entries, err := os.ReadDir(output)
	if err != nil || len(entries) != 1 || entries[0].Name() != "target" {
		t.Fatalf("失败后遗留暂存目录或准备锁: %v %v", entries, err)
	}
}

func TestRootlessGitArchive(t *testing.T) {
	for _, name := range []string{"./bin/git", "bin/git", "./../escape", "./bin/../../escape", "/bin/git", "././bin/git"} {
		t.Run(name, func(t *testing.T) {
			base := t.TempDir()
			archive := filepath.Join(base, "git.tar.gz")
			archiveFixture(t, archive, &tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0755, Size: 16})
			err := extract(t.Context(), archive, filepath.Join(base, "git"), ".")
			valid := name == "./bin/git" || name == "bin/git"
			if (err == nil) != valid {
				t.Fatalf("根路径处理错误: %v", err)
			}
		})
	}
}
