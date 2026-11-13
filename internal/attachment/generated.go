package attachment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/imageinput"
)

// saveGenerated 保留可信工具已经读取的固定内容；只对已受理执行写入会话存储。
// 源路径不再读取，文件与校验结果始终来自同一份字节。
func (s *Store) saveGenerated(ctx context.Context, conversationID, runID, kind, name string, data []byte) (model.Block, error) {
	if !validID(conversationID) || !validID(runID) || (kind != "image" && kind != "file") || name == "" || !ValidName(name) || len(data) > imageinput.MaxSourceBytes {
		return model.Block{}, model.ErrAttachment
	}
	exists, err := s.exists(ctx, conversationID, runID)
	if err != nil || !exists {
		return model.Block{}, errors.Join(model.ErrAttachment, err)
	}
	ext := strings.ToLower(filepath.Ext(name))
	if !validExtension(ext) {
		ext = ".bin"
	}
	mime := contentType(ext, data)
	if kind == "image" {
		if _, err := imageinput.Prepare(ctx, data, mime); err != nil {
			return model.Block{}, err
		}
		if extensions[mime] == "" {
			return model.Block{}, model.ErrImageFormat
		}
		ext = extensions[mime]
	}
	directory := filepath.Join(conversationID, runID)
	if err := s.root.MkdirAll(directory, 0700); err != nil {
		return model.Block{}, err
	}
	hash := sha256.Sum256(data)
	ref := filepath.ToSlash(filepath.Join(directory, hex.EncodeToString(hash[:])+ext))
	if err := s.write(ctx, filepath.FromSlash(ref), data); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return model.Block{}, err
		}
		if _, _, err := s.Read(ctx, ref); err != nil {
			return model.Block{}, err
		}
	}
	for _, name := range []string{directory, conversationID, "."} {
		if err := s.syncDirectory(name); err != nil {
			return model.Block{}, err
		}
	}
	return model.Block{Type: kind, AttachmentRef: ref, Name: name, MIME: mime, Size: int64(len(data))}, nil
}

// SaveGenerated 将本轮工具输出保留为视觉上下文或最终交付物，受本轮资源配额限制。
func (a *Access) SaveGenerated(ctx context.Context, conversationID, runID, kind, name string, data []byte, publish bool) (model.Block, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	run := a.runs[runID]
	if a.closed || run == nil || run.ConversationID != conversationID || run.bytes+int64(len(data)) > 128<<20 || len(run.files) >= 1024 || (publish && len(run.Artifacts) >= 63) {
		return model.Block{}, model.ErrAttachment
	}
	block, err := a.store.saveGenerated(ctx, conversationID, runID, kind, name, data)
	if err != nil {
		return block, err
	}
	key := "generated:" + block.AttachmentRef
	if _, exists := run.files[key]; !exists {
		run.files[key] = block.AttachmentRef
		run.bytes += int64(len(data))
	}
	if publish {
		for _, previous := range run.Artifacts {
			if previous.AttachmentRef == block.AttachmentRef {
				return block, nil
			}
		}
		run.Artifacts = append(run.Artifacts, block)
	}
	return block, nil
}
