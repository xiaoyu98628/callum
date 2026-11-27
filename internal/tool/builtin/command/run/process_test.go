package run

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/xiaoyu98628/callum/internal/tool"
)

func TestCleanupDiagnosticsPreserveErrorsWithoutSensitiveDetails(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		kind string
		code uint64
	}{
		{"system", &os.PathError{Op: "remove", Path: "private-secret-path", Err: syscall.EPERM}, "system", uint64(syscall.EPERM)},
		{"joined", errors.Join(errors.New("private-secret-message"), syscall.EACCES), "system", uint64(syscall.EACCES)},
		{"timeout", context.DeadlineExceeded, "timeout", 0},
		{"cancelled", context.Canceled, "cancelled", 0},
		{"closed", os.ErrClosed, "closed", 0},
		{"unavailable", tool.ErrUnavailable, "unavailable", 0},
		{"unknown", errors.New("private-secret-error"), "unknown", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := processReport{CleanupComplete: true}
			if err := report.cleanupError("execution_cleanup", tc.err); err != tc.err {
				t.Fatal("原始错误链被替换")
			}
			if report.CleanupComplete || len(report.CleanupErrors) != 1 {
				t.Fatalf("清理失败记录不符: %+v", report)
			}
			failure := report.CleanupErrors[0]
			if failure.Stage != "execution_cleanup" || failure.Kind != tc.kind || failure.SystemCode != tc.code {
				t.Fatalf("清理诊断不符: %+v", failure)
			}
			raw, err := json.Marshal(report)
			if err != nil || strings.Contains(string(raw), "private-secret") {
				t.Fatalf("诊断泄漏敏感详情: %s %v", raw, err)
			}
		})
	}
	clean := processReport{CleanupComplete: true}
	if err := clean.cleanupError("execution_cleanup", nil); err != nil || !clean.CleanupComplete {
		t.Fatal("成功收尾被改为失败")
	}
	raw, err := json.Marshal(clean)
	if err != nil || strings.Contains(string(raw), "cleanup_errors") {
		t.Fatalf("成功结果应省略失败诊断: %s %v", raw, err)
	}
}
