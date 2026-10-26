package logging

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
)

const backupTimeFormat = "20060102T150405.000000000Z"

type rotationPolicy struct {
	maxBytes    int64
	maxBackups  int
	maxAge      time.Duration
	lockTimeout time.Duration
}

type rotatingWriter struct {
	mu       sync.Mutex
	path     string
	lock     *flock.Flock
	policy   rotationPolicy
	closed   bool
	closeErr error
}

func newRotatingWriter(path string, policy rotationPolicy) (*rotatingWriter, error) {
	if path == "" || policy.maxBytes <= 0 || policy.maxBackups <= 0 || policy.maxAge <= 0 || policy.lockTimeout <= 0 {
		return nil, errors.New("日志路径或轮转配置无效")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0700); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}
	if err := checkRegularFile(absPath + ".lock"); err != nil {
		return nil, err
	}
	w := &rotatingWriter{
		path: absPath,
		// 锁文件长期保留，不能在退出时删除，否则不同进程可能锁住不同 inode。
		lock:   flock.New(absPath+".lock", flock.SetPermissions(0600)),
		policy: policy,
	}
	if err := w.withLock(func() error {
		file, err := w.openCurrent()
		if err != nil {
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
		return w.cleanup()
	}); err != nil {
		return nil, errors.Join(err, w.lock.Close())
	}
	return w, nil
}

func (w *rotatingWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, os.ErrClosed
	}
	if int64(len(data)) > w.policy.maxBytes {
		return 0, errors.New("单条日志超过文件大小限制")
	}
	var written int
	err := w.withLock(func() error {
		// 不能缓存文件句柄或大小：其他进程可能已经完成轮转。
		file, err := w.openCurrent()
		if err != nil {
			return err
		}
		info, err := file.Stat()
		if err != nil {
			return errors.Join(err, file.Close())
		}
		rotated := info.Size() > 0 && info.Size()+int64(len(data)) > w.policy.maxBytes
		if rotated {
			if err := file.Close(); err != nil {
				return err
			}
			if err := w.rotate(); err != nil {
				return err
			}
			file, err = w.openCurrent()
			if err != nil {
				return err
			}
		}
		written, err = file.Write(data)
		if err == nil && written != len(data) {
			err = io.ErrShortWrite
		}
		err = errors.Join(err, file.Close())
		if rotated {
			err = errors.Join(err, w.cleanup())
		}
		return err
	})
	return written, err
}

func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed {
		w.closed = true
		w.closeErr = w.lock.Close()
	}
	return w.closeErr
}

func (w *rotatingWriter) withLock(fn func() error) (operationErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), w.policy.lockTimeout)
	defer cancel()
	locked, err := w.lock.TryLockContext(ctx, 10*time.Millisecond)
	if err != nil {
		return fmt.Errorf("获取日志进程锁失败: %w", err)
	}
	if !locked {
		return errors.New("未取得日志进程锁")
	}
	defer func() { operationErr = errors.Join(operationErr, w.lock.Unlock()) }()
	return fn()
}

func checkRegularFile(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("日志路径必须指向普通文件: %s", path)
	}
	return nil
}

func (w *rotatingWriter) openCurrent() (*os.File, error) {
	if err := checkRegularFile(w.path); err != nil {
		return nil, err
	}
	return os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
}

func (w *rotatingWriter) rotate() error {
	ext := filepath.Ext(w.path)
	base := strings.TrimSuffix(w.path, ext)
	backup := base + "-" + time.Now().UTC().Format(backupTimeFormat) + "-" + rand.Text() + ext
	return os.Rename(w.path, backup)
}

type backupFile struct {
	name string
	time time.Time
}

// cleanup 必须持有与写入、轮转相同的锁，不使用后台清理协程。
func (w *rotatingWriter) cleanup() error {
	dir := filepath.Dir(w.path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var backups []backupFile
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		if created, ok := w.backupTime(entry.Name()); ok {
			backups = append(backups, backupFile{name: entry.Name(), time: created})
		}
	}
	sort.Slice(backups, func(i, j int) bool {
		if backups[i].time.Equal(backups[j].time) {
			return backups[i].name > backups[j].name
		}
		return backups[i].time.After(backups[j].time)
	})
	cutoff := time.Now().Add(-w.policy.maxAge)
	var cleanupErr error
	for index, backup := range backups {
		if index < w.policy.maxBackups && !backup.time.Before(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, backup.name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	return cleanupErr
}

func (w *rotatingWriter) backupTime(name string) (time.Time, bool) {
	ext := filepath.Ext(w.path)
	prefix := strings.TrimSuffix(filepath.Base(w.path), ext) + "-"
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ext) {
		return time.Time{}, false
	}
	stamp := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ext)
	if timestamp, suffix, ok := strings.Cut(stamp, "-"); ok && suffix != "" {
		if created, err := time.Parse(backupTimeFormat, timestamp); err == nil {
			return created, true
		}
	}
	// 升级后也识别旧 lumberjack 生成的 UTC 备份名。
	created, err := time.Parse("2006-01-02T15-04-05.000", stamp)
	return created, err == nil
}
