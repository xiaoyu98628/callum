// Package attachment 保管随会话提交的原始附件。
package attachment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/imageinput"
)

type Store struct {
	root    *os.Root
	dataDir string
	exists  func(context.Context, string, string) (bool, error)
	logger  *slog.Logger
}

// Open 在运行时实例锁内打开私有目录并恢复未完成导入。调用方负责 Close。
func Open(ctx context.Context, dataDir string, exists func(context.Context, string, string) (bool, error), logger *slog.Logger) (_ *Store, retErr error) {
	path := filepath.Join(dataDir, "attachments")
	if err := privateDirectory(path); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, root.Close())
		}
	}()
	if err := privateDirectory(filepath.Join(path, ".pending")); err != nil {
		return nil, err
	}
	s := &Store{root: root, dataDir: dataDir, exists: exists, logger: logger}
	if err := s.recover(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func privateDirectory(path string) error {
	if err := os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return errors.New("附件目录不是私有普通目录")
	}
	return nil
}

func (s *Store) Close() error { return s.root.Close() }

var extensions = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

func validID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 7 && id.String() == value
}

func validReference(ref, conversationID string) bool {
	parts := strings.Split(ref, "/")
	if len(parts) != 3 || !validID(parts[0]) || !validID(parts[1]) || (conversationID != "" && parts[0] != conversationID) {
		return false
	}
	ext := filepath.Ext(parts[2])
	if !validExtension(ext) {
		return false
	}
	hash := strings.TrimSuffix(parts[2], ext)
	data, err := hex.DecodeString(hash)
	return err == nil && len(data) == sha256.Size && hex.EncodeToString(data) == hash
}

// Read 校验引用及内容哈希，模型和预览共用这个读取边界。
func (s *Store) Read(ctx context.Context, ref string) (_ []byte, _ string, retErr error) {
	file, err := s.Pin(ctx, ref)
	if err != nil {
		return nil, "", err
	}
	defer func() { retErr = errors.Join(retErr, file.Close()) }()
	data, err := file.Read(ctx)
	if err != nil {
		return nil, "", err
	}
	return data, contentType(filepath.Ext(ref), data), nil
}

type Batch struct {
	store                 *Store
	conversationID, runID string
}

func (b *Batch) directory() string { return filepath.Join(b.conversationID, b.runID) }
func (b *Batch) marker() string    { return filepath.Join(".pending", b.conversationID+"_"+b.runID) }

// Stage 将原始附件落盘后返回副本引用。成功后调用方须根据 SQLite 受理结果收尾。
func (s *Store) Stage(ctx context.Context, conversationID, runID string, blocks []model.Block) (_ *Batch, retErr error) {
	if !validID(conversationID) || !validID(runID) {
		return nil, model.ErrImageRead
	}
	var batch *Batch
	defer func() {
		if retErr != nil && batch != nil {
			retErr = errors.Join(retErr, batch.Discard())
		}
	}()
	total, count := 0, 0
	for i, block := range blocks {
		if block.Type != "image" && block.Type != "file" {
			continue
		}
		count++
		if count > imageinput.MaxImages {
			return nil, model.ErrImageLimit
		}
		var data []byte
		var mime string
		var err error
		if block.AttachmentRef != "" {
			if !validReference(block.AttachmentRef, conversationID) {
				return nil, model.ErrImageRead
			}
			data, _, err = s.Read(ctx, block.AttachmentRef)
		} else {
			data, mime, err = s.readSource(ctx, block.Path, block.Type)
		}
		if err != nil {
			return nil, err
		}
		total += len(data)
		if total > imageinput.MaxSourceBatchBytes {
			return nil, model.ErrImageLimit
		}
		if block.AttachmentRef != "" {
			continue
		}
		if batch == nil {
			b := &Batch{s, conversationID, runID}
			if err := s.write(ctx, b.marker(), nil); err != nil {
				return nil, err
			}
			batch = b
			if err := s.syncDirectory(".pending"); err != nil {
				return nil, err
			}
			if err := s.root.MkdirAll(batch.directory(), 0700); err != nil {
				return nil, err
			}
		}
		name := block.Name
		if name == "" {
			name = filepath.Base(block.Path)
		}
		ext := extensions[mime]
		if block.Type == "file" {
			ext = strings.ToLower(filepath.Ext(name))
			if !validExtension(ext) {
				ext = ".bin"
			}
			mime = contentType(ext, data)
		}
		hash := sha256.Sum256(data)
		ref := filepath.ToSlash(filepath.Join(batch.directory(), hex.EncodeToString(hash[:])+ext))
		// 相同内容在本批只存一次，消息块仍保持用户选择的顺序。
		if err := s.write(ctx, filepath.FromSlash(ref), data); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		blocks[i] = model.Block{Type: block.Type, AttachmentRef: ref, Name: name, MIME: mime, Size: int64(len(data))}
	}
	if batch != nil {
		// 先固定文件与目录项，再允许 SQLite 提交引用。
		for _, dir := range []string{batch.directory(), conversationID, "."} {
			if err := s.syncDirectory(dir); err != nil {
				return nil, err
			}
		}
	}
	return batch, ctx.Err()
}

func (s *Store) write(ctx context.Context, path string, data []byte) (retErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := s.root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() {
		retErr = errors.Join(retErr, file.Close())
		if retErr != nil {
			retErr = errors.Join(retErr, s.root.Remove(path))
		}
	}()
	for len(data) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		part := data[:min(len(data), 256<<10)]
		if _, err := file.Write(part); err != nil {
			return err
		}
		data = data[len(part):]
	}
	return file.Sync()
}

func (s *Store) syncDirectory(path string) error {
	// Windows 文件句柄已同步，目录句柄不支持 Sync。
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := s.root.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}

func (b *Batch) Discard() error {
	if b == nil {
		return nil
	}
	if err := b.store.root.RemoveAll(b.directory()); err != nil {
		return err
	}
	// 保留会话目录。确认目录项已持久化后才移除恢复标记。
	if err := b.store.syncDirectory(b.conversationID); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return b.removeMarker()
}

func (b *Batch) removeMarker() error {
	err := b.store.root.Remove(b.marker())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Finish 在受理成功后只移除标记。提交结果未知时核对记录再回收。
func (b *Batch) Finish(ctx context.Context, accepted bool) {
	if b == nil {
		return
	}
	var err error
	if accepted {
		err = b.removeMarker()
	} else {
		check, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		err = b.recover(check)
	}
	if err != nil {
		b.store.logger.Warn("附件导入待恢复", "module", "attachments", "run_id", b.runID)
	}
}

func (b *Batch) recover(ctx context.Context) error {
	exists, err := b.store.exists(ctx, b.conversationID, b.runID)
	if err != nil {
		return err
	}
	if exists {
		return b.removeMarker()
	}
	return b.Discard()
}

func (s *Store) recover(ctx context.Context) (retErr error) {
	dir, err := s.root.Open(".pending")
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, dir.Close()) }()
	for {
		names, readErr := dir.Readdirnames(128)
		for _, name := range names {
			if err := ctx.Err(); err != nil {
				return err
			}
			conversationID, runID, ok := strings.Cut(name, "_")
			if !ok || !validID(conversationID) || !validID(runID) {
				return errors.New("附件恢复标记无效")
			}
			b := &Batch{s, conversationID, runID}
			if err := b.recover(ctx); err != nil {
				return err
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
	}
}
