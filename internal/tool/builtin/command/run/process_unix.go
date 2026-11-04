//go:build darwin || linux

package run

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"github.com/xiaoyu98628/callum/internal/tool"
)

type process struct {
	pid      int
	finished bool
	exit     processExit
}

func (p *process) processID() int { return p.pid }

// 元数据检查与打开之间可能发生目标替换，非阻塞打开避免 FIFO 挂住审批准备。
func openTargetPath(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}

func startProcess(fixed *fixedCommand, stdin, stdout, stderr *os.File) (*process, error) {
	pid, err := syscall.ForkExec(fixed.executable.actual, fixed.command, &syscall.ProcAttr{Dir: fixed.directory.actual, Env: fixed.environment, Files: []uintptr{stdin.Fd(), stdout.Fd(), stderr.Fd()}, Sys: &syscall.SysProcAttr{Setpgid: true}})
	if err != nil {
		return nil, err
	}
	return &process{pid: pid}, nil
}

func (p *process) poll() (processExit, bool, error) {
	if p.finished {
		return p.exit, true, nil
	}
	var status syscall.WaitStatus
	pid, err := syscall.Wait4(p.pid, &status, syscall.WNOHANG, nil)
	if errors.Is(err, syscall.EINTR) {
		return processExit{}, false, nil
	}
	if err != nil {
		return processExit{}, false, err
	}
	if pid == 0 {
		return processExit{}, false, nil
	}
	p.finished, p.exit.code = true, status.ExitStatus()
	if status.Signaled() {
		p.exit.signal = status.Signal().String()
	}
	return p.exit, true, nil
}

// stop 只作用于本次独立进程组，不使用进程名或遍历全局进程表。
// 主动 setsid/setpgid 脱离的后代不在此边界内；这不是操作系统沙箱。
func (p *process) stop() error {
	err := syscall.Kill(-p.pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
func (*process) close() error { return nil }

func fileIdentity(_ *os.File, info os.FileInfo) (string, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", tool.ErrUnavailable
	}
	return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino), nil
}
