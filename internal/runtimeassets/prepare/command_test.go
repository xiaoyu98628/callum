package prepare

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/runtimeassets"
)

func TestOfflineCommandCannotEnableDownloads(t *testing.T) {
	directory := t.TempDir()
	source := Source{Version: "fixed", URL: "https://example.invalid/archive", SHA256: strings.Repeat("0", 64), Archive: "missing.tar.gz", Prefix: "root", Entry: "bin/interpreter"}
	data, err := json.Marshal(sources{FormatVersion: runtimeassets.FormatVersion, Targets: []Target{{Directory: "target", Platform: runtime.GOOS, Arch: runtime.GOARCH, Python: source, Node: source, Git: GitSource{Source: source, ExecPath: "libexec", TemplatePath: "templates", ConfigPath: "etc/gitconfig"}}}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "sources.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(directory, "unpacked")
	args := []string{"--sources", path, "--output", output, "--cache", directory, "--download"}
	err = RunCommand(t.Context(), args, slog.New(slog.NewJSONHandler(io.Discard, nil)), true)
	if err == nil || !strings.Contains(err.Error(), "仅支持离线归档") {
		t.Fatalf("客户端准备入口接受了下载参数: %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("拒绝参数前创建了资源目录: %v", err)
	}
}
