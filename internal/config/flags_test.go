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
	cfg, err := LoadArgs([]string{"--data-dir", dir, "--max-concurrent-runs", "2", "--call-timeout", "30s", "--approval-policy", "always", "--approvals-reviewer", "user", "--tool-timeout", "10s"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != dir || cfg.MaxConcurrentRuns != 2 || cfg.CallTimeout != 30*time.Second || cfg.ApprovalPolicy != "always" || cfg.ApprovalsReviewer != "user" || cfg.ToolTimeout != 10*time.Second {
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

func TestApprovalFlagsRemainIndependent(t *testing.T) {
	for _, choice := range []string{"on-request", "always", "never"} {
		cfg, err := LoadArgs([]string{"--data-dir", t.TempDir(), "--approval-policy", choice})
		if err != nil || string(cfg.ApprovalPolicy) != choice || cfg.ApprovalsReviewer != "user" {
			t.Fatalf("策略改变了审批人: %+v %v", cfg, err)
		}
	}
	cfg, err := LoadArgs([]string{"--data-dir", t.TempDir(), "--approvals-reviewer", "user"})
	if err != nil || cfg.ApprovalPolicy != "on-request" {
		t.Fatalf("审批人改变了默认策略: %+v %v", cfg, err)
	}
	var usage bytes.Buffer
	WriteUsage(&usage)
	for _, name := range []string{"-approval-policy", "-approvals-reviewer", "on-request", "never"} {
		if !strings.Contains(usage.String(), name) {
			t.Fatalf("帮助缺少 %q: %s", name, usage.String())
		}
	}
	if strings.Contains(usage.String(), "-approval-mode") || strings.Contains(usage.String(), "-sandbox") {
		t.Fatal("帮助仍含旧配置或未实现的沙箱配置")
	}
}
