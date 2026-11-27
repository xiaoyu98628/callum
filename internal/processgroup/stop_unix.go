//go:build darwin || linux

// Package processgroup 统一独立 Unix 进程组的停止与有界收尾确认，不持有进程或管道。
package processgroup

import (
	"context"
	"errors"
	"runtime"
	"syscall"
	"time"
)

// Stop 只向调用者持有的独立进程组发送一次终止信号。
// exited 必须来自已经完成的主进程等待，不能用 PID 存在性推测。
func Stop(pid int, exited bool, timeout time.Duration) error {
	if pid <= 0 || timeout <= 0 {
		return syscall.EINVAL
	}
	return stop(exited, runtime.GOOS == "darwin", timeout, func(signal syscall.Signal) error { return syscall.Kill(-pid, signal) })
}

func stop(exited, darwin bool, timeout time.Duration, signal func(syscall.Signal) error) error {
	err := signal(syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	if !darwin || !exited || !errors.Is(err, syscall.EPERM) {
		return err
	}
	// macOS 对退出中的空进程组也可能返回 EPERM。之后只读确认消失，
	// 不再次向已回收的组号发送终止信号；无法确认时保留原始错误。
	if confirmErr := waitRemoved(func() error { return signal(0) }, timeout); confirmErr != nil {
		return errors.Join(err, confirmErr)
	}
	return nil
}

func waitRemoved(probe func() error, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := probe()
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if err != nil && !errors.Is(err, syscall.EPERM) {
			return err
		}
		select {
		case <-deadline.C:
			return context.DeadlineExceeded
		case <-ticker.C:
		}
	}
}
