//go:build darwin || linux

package main

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

func fileFlags(t *testing.T, file *os.File) uintptr {
	t.Helper()
	raw, err := file.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var flags uintptr
	var operationErr error
	if err := raw.Control(func(fd uintptr) {
		var errno syscall.Errno
		flags, _, errno = syscall.Syscall(syscall.SYS_FCNTL, fd, syscall.F_GETFL, 0)
		if errno != 0 {
			operationErr = errno
		}
	}); err != nil {
		t.Fatal(err)
	}
	if operationErr != nil {
		t.Fatal(operationErr)
	}
	return flags
}

func TestInheritedStdioCanBeInterruptedAndRestoresFlags(t *testing.T) {
	for _, writing := range []bool{false, true} {
		t.Run(map[bool]string{false: "读取", true: "写入"}[writing], func(t *testing.T) {
			var fds [2]int
			if err := syscall.Pipe(fds[:]); err != nil {
				t.Fatal(err)
			}
			reader := os.NewFile(uintptr(fds[0]), "inherited-reader")
			writer := os.NewFile(uintptr(fds[1]), "inherited-writer")
			t.Cleanup(func() {
				if err := reader.Close(); err != nil {
					t.Error(err)
				}
				if err := writer.Close(); err != nil {
					t.Error(err)
				}
			})
			original := reader
			if writing {
				original = writer
			}
			before := fileFlags(t, original)
			wrapped, err := newPollableFile(original)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := wrapped.Close(); err != nil {
					t.Error(err)
				}
			})
			if err := wrapped.SetDeadline(time.Now().Add(time.Hour)); err != nil {
				t.Fatalf("继承描述符未注册到运行时轮询器: %v", err)
			}
			started := make(chan struct{})
			done := make(chan error, 1)
			go func() {
				close(started)
				if writing {
					_, err := wrapped.Write([]byte(strings.Repeat("x", 8<<20)))
					done <- err
				} else {
					_, err := wrapped.Read(make([]byte, 1))
					done <- err
				}
			}()
			<-started
			if err := wrapped.Close(); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if !errors.Is(err, os.ErrClosed) {
					t.Fatalf("关闭未中断 I/O: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("关闭后 I/O 仍然阻塞")
			}
			if got := fileFlags(t, original); got != before {
				t.Fatalf("未恢复原始文件状态: before=%d after=%d", before, got)
			}
			if _, err := original.Stat(); err != nil {
				t.Fatalf("借用的原始描述符被关闭: %v", err)
			}
		})
	}
}

func TestSharedStdioFlagsRestoreAfterBothStreamsClose(t *testing.T) {
	var fds [2]int
	if err := syscall.Pipe(fds[:]); err != nil {
		t.Fatal(err)
	}
	reader := os.NewFile(uintptr(fds[0]), "shared-reader")
	writer := os.NewFile(uintptr(fds[1]), "writer")
	t.Cleanup(func() {
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
		if err := writer.Close(); err != nil {
			t.Error(err)
		}
	})
	before := fileFlags(t, reader)
	in, err := newPollableFile(reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := in.Close(); err != nil {
			t.Error(err)
		}
	})
	out, err := newPollableFile(reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := out.Close(); err != nil {
			t.Error(err)
		}
	})
	restoreTogether(in, out)
	if err := in.Close(); err != nil {
		t.Fatal(err)
	}
	if fileFlags(t, reader)&syscall.O_NONBLOCK == 0 {
		t.Fatal("另一个副本仍持有文件时过早恢复了阻塞模式")
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if fileFlags(t, reader) != before {
		t.Fatal("两个副本关闭后未恢复原始标志")
	}
}
