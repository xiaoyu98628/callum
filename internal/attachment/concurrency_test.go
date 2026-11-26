package attachment

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/xiaoyu98628/callum/internal/model"
)

func accessFixture(t *testing.T, exists func(context.Context, string, string) (bool, error)) *Access {
	t.Helper()
	directory := t.TempDir()
	store, err := Open(t.Context(), directory, exists, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	access, err := OpenAccess(t.Context(), directory, store, func(context.Context, string, string) (model.Block, error) {
		return model.Block{}, model.ErrAttachment
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := access.Close(); err != nil {
			t.Error(err)
		}
	})
	return access
}

func TestAccessIsolatesRunsAndWaitsBeforeCleanup(t *testing.T) {
	for _, closeAll := range []bool{false, true} {
		t.Run(fmt.Sprintf("close=%t", closeAll), func(t *testing.T) {
			conv, slow, other := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
			started, release := make(chan struct{}), make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })
			access := accessFixture(t, func(_ context.Context, _, runID string) (bool, error) {
				if runID == slow {
					close(started)
					<-release
				}
				return true, nil
			})
			defer unblock()
			files, err := access.Begin(t.Context(), conv, slow)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := access.Begin(t.Context(), conv, other); err != nil {
				t.Fatal(err)
			}
			finished := make(chan error, 1)
			go func() {
				_, err := access.SaveGenerated(t.Context(), conv, slow, "file", "slow.txt", []byte("slow"), true)
				finished <- err
			}()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("附件操作未开始")
			}
			// 一个执行的归属查询被阻塞时，另一个执行仍能查询、生成和回收。
			independent := make(chan error, 1)
			go func() {
				if _, ok := access.Files(conv, other); !ok {
					independent <- errors.New("独立执行不可查询")
					return
				}
				_, err := access.SaveGenerated(t.Context(), conv, other, "file", "other.txt", []byte("other"), true)
				independent <- errors.Join(err, access.End(other))
			}()
			select {
			case err := <-independent:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("另一个执行被全局附件锁阻塞")
			}
			// 等待同一执行的操作可以取消，不需要等当前文件操作结束。
			ctx, cancel := context.WithCancel(t.Context())
			waiting := make(chan error, 1)
			go func() {
				_, err := access.SaveGenerated(ctx, conv, slow, "file", "waiting.txt", nil, true)
				waiting <- err
			}()
			cancel()
			select {
			case err := <-waiting:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("等待未保留取消: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("等待文件操作不能取消")
			}
			cleaned := make(chan error, 1)
			go func() {
				if closeAll {
					cleaned <- access.Close()
				} else {
					cleaned <- access.End(slow)
				}
			}()
			select {
			case err := <-cleaned:
				t.Fatalf("操作尚未完成便回收: %v", err)
			case <-time.After(30 * time.Millisecond):
			}
			if _, err := os.Stat(files.Work); err != nil {
				t.Fatalf("在途操作的目录已被回收: %v", err)
			}
			unblock()
			if err := <-finished; err != nil {
				t.Fatal(err)
			}
			if err := <-cleaned; err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(files.Work); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("操作结束后目录未回收: %v", err)
			}
			if _, err := os.Stat(access.marker(slow)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("恢复标记未回收: %v", err)
			}
			if err := access.End(slow); err != nil {
				t.Fatal(err)
			}
			if !closeAll {
				if _, err := access.Begin(t.Context(), conv, slow); err != nil {
					t.Fatalf("同 ID 的后续租约不能重新建立: %v", err)
				}
			}
		})
	}
}

func TestConcurrentPublishingKeepsArtifactQuota(t *testing.T) {
	access := accessFixture(t, func(context.Context, string, string) (bool, error) { return true, nil })
	conv, run := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
	if _, err := access.Begin(t.Context(), conv, run); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	var succeeded atomic.Int32
	for i := range 65 {
		workers.Go(func() {
			_, err := access.SaveGenerated(t.Context(), conv, run, "file", fmt.Sprintf("%d.txt", i), []byte(fmt.Sprint(i)), true)
			if err == nil {
				succeeded.Add(1)
			} else if !errors.Is(err, model.ErrAttachment) {
				t.Errorf("发布失败: %v", err)
			}
		})
	}
	workers.Wait()
	files, ok := access.Files(conv, run)
	if !ok || len(files.Artifacts) != 63 || succeeded.Load() != 63 {
		t.Fatalf("并发发布突破或丢失配额: %d %d", len(files.Artifacts), succeeded.Load())
	}
	files.Artifacts[0].Name = "changed"
	again, _ := access.Files(conv, run)
	if again.Artifacts[0].Name == "changed" {
		t.Fatal("交付物快照仍借用内部切片")
	}
}

func TestAccessRetriesFailedInitializationAndCleanup(t *testing.T) {
	access := accessFixture(t, func(context.Context, string, string) (bool, error) { return true, nil })
	conv, run := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
	marker := access.marker(run)
	if err := os.WriteFile(marker, []byte(`"existing"`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := access.Begin(t.Context(), conv, run); err == nil {
		t.Fatal("初始化覆盖了已有恢复标记")
	}
	if _, ok := access.Files(conv, run); ok {
		t.Fatal("失败初始化暴露了工作目录")
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	files, err := access.Begin(t.Context(), conv, run)
	if err != nil {
		t.Fatalf("初始化失败后仍占用执行槽位: %v", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(marker, 0700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(marker, "block-removal")
	if err := os.WriteFile(child, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := access.End(run); err == nil {
		t.Fatal("清理失败被忽略")
	}
	if _, ok := access.Files(conv, run); ok {
		t.Fatal("清理失败后仍接受新操作")
	}
	if err := os.Remove(child); err != nil {
		t.Fatal(err)
	}
	if err := access.End(run); err != nil {
		t.Fatalf("不能重试清理: %v", err)
	}
	if _, err := os.Stat(files.Work); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("工作目录未回收: %v", err)
	}
	if err := access.Close(); err != nil {
		t.Fatal(err)
	}
	if err := access.Close(); err != nil {
		t.Fatal(err)
	}
}
