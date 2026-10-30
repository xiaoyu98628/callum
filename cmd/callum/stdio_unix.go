//go:build darwin || linux

package main

import (
	"errors"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
)

// pollableFile 持有标准流的副本，借用原始描述符以恢复共享的非阻塞标志。
// 原始标准流在副本关闭之前必须保持打开，main 在整个运行期间持有它们。
type pollableFile struct {
	*os.File
	original        *os.File
	restoreBlocking bool
	once            sync.Once
	closeErr        error
	afterClose      func() error
}

func newPollableFile(original *os.File) (*pollableFile, error) {
	raw, err := original.SyscallConn()
	if err != nil {
		return nil, err
	}
	duplicate := -1
	var operationErr error
	var restore bool
	controlErr := raw.Control(func(fd uintptr) {
		flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, fd, syscall.F_GETFL, 0)
		if errno != 0 {
			operationErr = errno
			return
		}
		restore = flags&syscall.O_NONBLOCK == 0
		duplicate, operationErr = syscall.Dup(int(fd))
		if operationErr != nil {
			return
		}
		syscall.CloseOnExec(duplicate)
		operationErr = syscall.SetNonblock(duplicate, true)
	})
	if err := errors.Join(controlErr, operationErr); err != nil {
		if duplicate >= 0 {
			err = errors.Join(err, syscall.Close(duplicate))
		}
		return nil, err
	}
	// NewFile 只有接收到非阻塞描述符才会将继承的终端或管道注册到运行时轮询器。
	// 此后 Close 可以解除等待，避免 Ctrl+C 或写入超时被阻塞系统调用拖住。
	return &pollableFile{File: os.NewFile(uintptr(duplicate), original.Name()), original: original, restoreBlocking: restore}, nil
}

func (f *pollableFile) Close() error {
	f.once.Do(func() {
		// 先关闭副本、等待其读写退出，再恢复标志，防止正在读取的副本重新变成阻塞模式。
		f.closeErr = f.File.Close()
		if f.afterClose != nil {
			f.closeErr = errors.Join(f.closeErr, f.afterClose())
		} else {
			f.closeErr = errors.Join(f.closeErr, f.restore())
		}
	})
	return f.closeErr
}

func (f *pollableFile) restore() error {
	if !f.restoreBlocking {
		return nil
	}
	raw, err := f.original.SyscallConn()
	if err != nil {
		return err
	}
	var restoreErr error
	controlErr := raw.Control(func(fd uintptr) { restoreErr = syscall.SetNonblock(int(fd), false) })
	return errors.Join(controlErr, restoreErr)
}

func restoreTogether(in, out *pollableFile) {
	var remaining atomic.Int32
	remaining.Store(2)
	// 终端的 stdin/stdout 可能共享同一文件状态，必须等两个副本都停止读写后再恢复。
	afterClose := func() error {
		if remaining.Add(-1) == 0 {
			return errors.Join(in.restore(), out.restore())
		}
		return nil
	}
	in.afterClose, out.afterClose = afterClose, afterClose
}

func openStdio() (io.ReadCloser, io.WriteCloser, error) {
	in, err := newPollableFile(os.Stdin)
	if err != nil {
		return nil, nil, err
	}
	out, err := newPollableFile(os.Stdout)
	if err != nil {
		return nil, nil, errors.Join(err, in.Close())
	}
	restoreTogether(in, out)
	return in, out, nil
}
