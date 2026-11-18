//go:build unix

package imageinput

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/model"
)

func TestImageFileReplacedByFIFODoesNotBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.png")
	if err := os.WriteFile(path, []byte("image-content"), 0600); err != nil {
		t.Fatal(err)
	}
	expected, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	// 确定性地在检查与打开之间替换目标，不依赖高频路径竞争。
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := readFile(ctx, func() (*os.File, error) { return os.OpenFile(path, readFlags, 0) }, expected)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, model.ErrImageRead) {
			t.Fatalf("未拒绝被替换的图片: %v", err)
		}
	case <-ctx.Done():
		// 回归时用写端解除阻塞的 open，确保失败测试也不遗留读取 goroutine。
		writer, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			t.Error(err)
		} else if err := writer.Close(); err != nil {
			t.Error(err)
		}
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("解除命名管道阻塞后读取仍未退出")
		}
		t.Fatal("命名管道替换导致图片打开阻塞，取消无法结束")
	}
}
