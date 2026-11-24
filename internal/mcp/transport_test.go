package mcp

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestChildProcessClose(t *testing.T) {
	stopErr := errors.New("停止子进程失败")
	for _, tc := range []struct {
		name        string
		alreadyDone bool
		stopErr     error
		elapsed     time.Duration
	}{
		{name: "自然退出后仍清理进程组", alreadyDone: true},
		{name: "强制停止成功", elapsed: processExitGrace},
		{name: "停止失败且未退出", stopErr: stopErr, elapsed: processExitGrace + processStopWait},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				done := make(chan struct{})
				if tc.alreadyDone {
					close(done)
				}
				var stops int
				p := &childProcess{in: pipeEnd(t, true), out: pipeEnd(t, false), done: done, stop: func() error {
					stops++
					if !tc.alreadyDone && tc.stopErr == nil {
						close(done)
					}
					return tc.stopErr
				}}
				started := time.Now()
				err := p.Close()
				if elapsed := time.Since(started); elapsed != tc.elapsed {
					t.Errorf("关闭耗时 %v，期望 %v", elapsed, tc.elapsed)
				}
				if tc.stopErr != nil {
					if !errors.Is(err, stopErr) || !errors.Is(err, context.DeadlineExceeded) {
						t.Errorf("关闭应保留停止失败和超时，得到 %v", err)
					}
					select {
					case <-done:
						t.Error("测试进程应仍在运行")
					default:
					}
				} else if err != nil {
					t.Errorf("关闭失败: %v", err)
				}
				if p.Close() != err || stops != 1 {
					t.Errorf("重复关闭应复用结果，停止调用次数 %d", stops)
				}
				for _, f := range []*os.File{p.in, p.out} {
					if _, err := f.Stat(); !errors.Is(err, os.ErrClosed) {
						t.Errorf("关闭后管道仍可访问: %v", err)
					}
				}
			})
		})
	}
}

func TestChildProcessConcurrentClose(t *testing.T) {
	done := make(chan struct{})
	close(done)
	stopErr := errors.New("清理进程组失败")
	var stops atomic.Int32
	p := &childProcess{in: pipeEnd(t, true), out: pipeEnd(t, false), done: done, stop: func() error {
		stops.Add(1)
		return stopErr
	}}
	start := make(chan struct{})
	results := make(chan error, 8)
	for range cap(results) {
		go func() { <-start; results <- p.Close() }()
	}
	close(start)
	first := <-results
	if !errors.Is(first, stopErr) {
		t.Errorf("关闭应返回停止错误，得到 %v", first)
	}
	for range cap(results) - 1 {
		if err := <-results; err != first {
			t.Errorf("并发关闭返回了不同的结果: %v", err)
		}
	}
	if stops.Load() != 1 {
		t.Errorf("并发关闭多次调用停止操作: %d", stops.Load())
	}
}

func pipeEnd(t *testing.T, write bool) *os.File {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})
	if write {
		return w
	}
	return r
}
