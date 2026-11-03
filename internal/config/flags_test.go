package config

import (
	"bytes"
	"errors"
	"flag"
	"strings"
	"testing"
	"time"
)

func TestRuntimeFlags(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadArgs([]string{"--data-dir", dir, "--max-concurrent-runs", "2", "--call-timeout", "30s", "--tool-timeout", "10s"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != dir || cfg.MaxConcurrentRuns != 2 || cfg.CallTimeout != 30*time.Second || cfg.ToolTimeout != 10*time.Second {
		t.Fatalf("参数未生效: %+v", cfg)
	}
	for _, args := range [][]string{{"--max-concurrent-runs", "0"}, {"--max-concurrent-runs", "65"}, {"--call-timeout", "0s"}, {"--call-timeout", "bad"}, {"unknown"}, {"--unknown"}, {"--approval-mode", "always"}, {"--approval-policy", "auto-read"}, {"--approval-policy", "bad"}, {"--approval-policy", ""}, {"--approvals-reviewer", "auto_review"}, {"--approvals-reviewer", ""}, {"--approval-timeout", "1m"}, {"--tool-timeout", "-1s"}} {
		if _, err := LoadArgs(args); err == nil {
			t.Fatalf("错误参数被接受: %v", args)
		}
	}
	if _, err := LoadArgs([]string{"--help"}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("帮助错误: %v", err)
	}
}

func TestApprovalSettingsAreNotStartupFlags(t *testing.T) {
	for _, args := range [][]string{{"--approval-policy", "on-request"}, {"--approvals-reviewer", "user"}, {"--approval-mode", "always"}} {
		if _, err := LoadArgs(append([]string{"--data-dir", t.TempDir()}, args...)); err == nil {
			t.Fatalf("接受已迁入数据库的启动参数: %v", args)
		}
	}
	var usage bytes.Buffer
	WriteUsage(&usage)
	for _, name := range []string{"-approval-policy", "-approvals-reviewer", "-approval-mode", "-sandbox"} {
		if strings.Contains(usage.String(), name) {
			t.Fatalf("帮助仍含无效参数 %s", name)
		}
	}
}
