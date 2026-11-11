package attachment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/imageinput"
)

// ValidName 允许未指定名称。显示名称使用单个文件名，不能包含控制字符或目录。
func ValidName(name string) bool {
	return utf8.ValidString(name) && len(name) <= 1024 &&
		!strings.ContainsAny(name, "/\\") &&
		!strings.ContainsFunc(name, unicode.IsControl) && name != "." && name != ".."
}

func validExtension(ext string) bool {
	if len(ext) < 2 || len(ext) > 17 || ext[0] != '.' {
		return false
	}
	for _, c := range ext[1:] {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

func contentType(ext string, data []byte) string {
	if expected := mime.TypeByExtension(ext); expected != "" {
		return strings.SplitN(expected, ";", 2)[0]
	}
	return strings.SplitN(http.DetectContentType(data), ";", 2)[0]
}

// File 固定一次读取的文件身份及内容哈希，持有者负责关闭。
type File struct {
	file     *os.File
	expected os.FileInfo
	ref      string
}

func (s *Store) Pin(ctx context.Context, ref string) (*File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validReference(ref, "") {
		return nil, model.ErrAttachment
	}
	path := filepath.FromSlash(ref)
	info, err := s.root.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > imageinput.MaxSourceBytes {
		return nil, model.ErrAttachment
	}
	file, err := s.root.OpenFile(path, readFlags, 0)
	if err != nil {
		return nil, errors.Join(model.ErrAttachment, err)
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
		return nil, errors.Join(model.ErrAttachment, err, file.Close())
	}
	return &File{file: file, expected: opened, ref: ref}, nil
}

func (f *File) Read(ctx context.Context) ([]byte, error) {
	info, err := f.file.Stat()
	if err != nil || !os.SameFile(info, f.expected) || info.Size() > imageinput.MaxSourceBytes {
		return nil, model.ErrAttachment
	}
	data, err := readBytes(ctx, io.NewSectionReader(f.file, 0, info.Size()+1))
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != strings.TrimSuffix(filepath.Base(f.ref), filepath.Ext(f.ref)) {
		return nil, model.ErrAttachment
	}
	return data, nil
}

func (f *File) Close() error { return f.file.Close() }

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func readBytes(ctx context.Context, reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(contextReader{ctx, reader}, imageinput.MaxSourceBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > imageinput.MaxSourceBytes {
		return nil, model.ErrAttachment
	}
	return data, ctx.Err()
}

func (s *Store) readSource(ctx context.Context, path, kind string) (_ []byte, _ string, retErr error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, "", model.ErrAttachment
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, "", model.ErrAttachment
	}
	relative, relErr := filepath.Rel(strings.ToLower(s.dataDir), strings.ToLower(canonical))
	if relErr == nil && filepath.IsLocal(relative) {
		parts := strings.Split(relative, string(filepath.Separator))
		if len(parts) != 3 || parts[0] != ".attachment-staging" || !strings.HasPrefix(parts[1], "upload-") {
			return nil, "", model.ErrAttachment
		}
	}
	file, err := os.OpenFile(canonical, readFlags, 0)
	if err != nil {
		return nil, "", model.ErrAttachment
	}
	defer func() { retErr = errors.Join(retErr, file.Close()) }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > imageinput.MaxSourceBytes {
		return nil, "", model.ErrAttachment
	}
	for _, suffix := range []string{"", "-wal", "-shm", "-journal", ".lock"} {
		dbInfo, err := os.Stat(filepath.Join(s.dataDir, "callum.db") + suffix)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, "", err
		}
		if err == nil && os.SameFile(info, dbInfo) {
			return nil, "", model.ErrAttachment
		}
	}
	data, err := readBytes(ctx, file)
	if err != nil {
		return nil, "", err
	}
	detected := strings.SplitN(http.DetectContentType(data), ";", 2)[0]
	if kind == "image" && extensions[detected] == "" {
		return nil, "", model.ErrImageFormat
	}
	if kind == "image" && len(data) == 0 {
		return nil, "", model.ErrImageFormat
	}
	if kind == "file" && bytes.HasPrefix(data, []byte("SQLite format 3\x00")) {
		// SQLite 身份校验之外，也拒绝导入数据库副本。
		return nil, "", model.ErrAttachment
	}
	return data, detected, nil
}
