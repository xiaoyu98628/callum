//go:build darwin || linux

package mcp

import (
	"errors"
	"os/exec"
	"syscall"
)

func startProcess(cmd *exec.Cmd) (func() error, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	pid := cmd.Process.Pid
	return func() error {
		err := syscall.Kill(-pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}, nil
}
