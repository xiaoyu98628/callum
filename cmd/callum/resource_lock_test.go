package main

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

func TestResourceLockExcludesServiceWithoutOpeningDatabase(t *testing.T) {
	database := filepath.Join(t.TempDir(), "private", "callum.db")
	input, writer := io.Pipe()
	reader, output := io.Pipe()
	defer writer.Close()
	defer reader.Close()
	done := make(chan error, 1)
	go func() { done <- holdResourceLock(t.Context(), database, input, output) }()
	scanner := bufio.NewScanner(reader)
	if !scanner.Scan() || !strings.Contains(scanner.Text(), `"resource_lock":true`) {
		t.Fatalf("资源锁未就绪: %s %v", scanner.Text(), <-done)
	}
	if _, err := os.Stat(database); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("资源锁创建了业务数据库: %v", err)
	}
	if owner, err := sqlite.AcquireOwnership(database); err == nil {
		owner.Close()
		t.Fatal("服务绕过了资源安装锁")
	}
	if _, err := io.WriteString(writer, "{\"jsonrpc\":\"2.0\",\"id\":\"validation\",\"method\":\"runtime.validate\",\"params\":{\"name\":\"runtimes\"}}\n"); err != nil {
		t.Fatal(err)
	}
	if !scanner.Scan() || !strings.Contains(scanner.Text(), `"valid":false`) {
		t.Fatalf("缺失资源未报告校验失败: %s", scanner.Text())
	}
	if _, err := io.WriteString(writer, `{"jsonrpc":"2.0","id":"pinned","method":"runtime.validate","params":{"name":"runtimes","manifest_sha256":"`+strings.Repeat("a", 64)+`"}}`+"\n"); err != nil {
		t.Fatal(err)
	}
	if !scanner.Scan() || !strings.Contains(scanner.Text(), `"valid":false`) {
		t.Fatalf("带摘要的校验未报告资源缺失: %s", scanner.Text())
	}
	for _, digest := range []string{`""`, `null`, `"wrong"`, `"` + strings.Repeat("A", 64) + `"`} {
		if _, err := io.WriteString(writer, `{"jsonrpc":"2.0","id":"bad-digest","method":"runtime.validate","params":{"name":"runtimes","manifest_sha256":`+digest+`}}`+"\n"); err != nil {
			t.Fatal(err)
		}
		if !scanner.Scan() || !strings.Contains(scanner.Text(), `"code":-32602`) {
			t.Fatalf("未拒绝无效的清单摘要 %s: %s", digest, scanner.Text())
		}
	}
	if _, err := io.WriteString(writer, "{\"jsonrpc\":\"2.0\",\"id\":\"invalid\",\"method\":\"runtime.validate\",\"params\":{\"name\":\"../runtimes\"}}\n"); err != nil {
		t.Fatal(err)
	}
	if !scanner.Scan() || !strings.Contains(scanner.Text(), `"code":-32602`) {
		t.Fatalf("资源校验请求越过数据目录: %s", scanner.Text())
	}
	if _, err := io.WriteString(writer, "{\"jsonrpc\":\"2.0\",\"id\":\"x\",\"method\":\"system.shutdown\",\"params\":{}}\n"); err != nil {
		t.Fatal(err)
	}
	if !scanner.Scan() || !strings.Contains(scanner.Text(), `"shutdown":true`) {
		t.Fatal("资源锁关闭未响应")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	owner, err := sqlite.AcquireOwnership(database)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
}
