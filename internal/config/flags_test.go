package config

import (
	"errors"
	"flag"
	"testing"
	"time"
)

func TestRuntimeFlags(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadArgs([]string{"--data-dir", dir, "--max-concurrent-runs", "2", "--call-timeout", "30s"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != dir || cfg.MaxConcurrentRuns != 2 || cfg.CallTimeout != 30*time.Second {
		t.Fatalf("参数未生效: %+v", cfg)
	}
	for _, args := range [][]string{{"--max-concurrent-runs", "0"}, {"--max-concurrent-runs", "65"}, {"--call-timeout", "0s"}, {"--call-timeout", "bad"}, {"unknown"}, {"--unknown"}} {
		if _, err := LoadArgs(args); err == nil {
			t.Fatalf("错误参数被接受: %v", args)
		}
	}
	if _, err := LoadArgs([]string{"--help"}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("帮助错误: %v", err)
	}
}
