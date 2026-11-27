//go:build darwin || linux

package mcp

import (
	"os/exec"
	"testing"
)

func TestExitedMCPProcessGroupCanBeClosed(t *testing.T) {
	for range 20 {
		done := make(chan struct{})
		cmd := exec.CommandContext(t.Context(), "/usr/bin/true")
		stop, err := startProcess(cmd, done)
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
		close(done)
		if err := stop(); err != nil {
			t.Fatalf("退出后无法清理进程组: %v", err)
		}
	}
}
