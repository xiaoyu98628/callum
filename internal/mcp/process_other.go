//go:build !darwin && !linux && !windows

package mcp

import "os/exec"

func startProcess(*exec.Cmd, <-chan struct{}) (func() error, error) { return nil, ErrConnection }
