//go:build darwin || linux

package mcp

import (
	"os/exec"
	"syscall"

	"github.com/xiaoyu98628/callum/internal/processgroup"
)

func startProcess(cmd *exec.Cmd, done <-chan struct{}) (func() error, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	pid := cmd.Process.Pid
	return func() error {
		exited := false
		select {
		case <-done:
			exited = true
		default:
		}
		return processgroup.Stop(pid, exited, processStopWait)
	}, nil
}
