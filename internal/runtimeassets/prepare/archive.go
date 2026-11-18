package prepare

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func safePath(name string) bool {
	return name != "." && fs.ValidPath(name) && !strings.ContainsAny(name, `\:`) && !strings.ContainsRune(name, 0)
}

type unpacker struct {
	ctx   context.Context
	root  *os.Root
	bytes int64
	count int
	seen  map[string]bool
	links map[string]string
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}

func (u *unpacker) entry(name string, size int64, mode fs.FileMode, reader io.Reader, link string) error {
	if err := u.ctx.Err(); err != nil {
		return err
	}
	if !safePath(name) || size < 0 || u.count >= 30000 || u.bytes+size > 2<<30 {
		return errors.New("归档路径或资源边界无效")
	}
	u.count++
	key := strings.ToLower(name)
	if u.seen[key] && !mode.IsDir() {
		return errors.New("归档包含重复路径")
	}
	u.seen[key] = true
	u.bytes += size
	if mode.IsDir() {
		return u.root.MkdirAll(filepath.FromSlash(name), 0755)
	}
	if err := u.root.MkdirAll(filepath.FromSlash(path.Dir(name)), 0755); err != nil {
		return err
	}
	if mode&os.ModeSymlink != 0 {
		if link == "" || strings.ContainsAny(link, `\:`) || path.IsAbs(link) || !safePath(path.Join(path.Dir(name), link)) {
			return errors.New("归档链接越出组件目录")
		}
		u.links[name] = link
		return nil
	}
	if !mode.IsRegular() {
		return errors.New("归档包含不受支持的文件类型")
	}
	file, err := u.root.OpenFile(filepath.FromSlash(name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644|mode.Perm()&0111)
	if err != nil {
		return err
	}
	_, copyErr := io.CopyN(file, contextReader{ctx: u.ctx, reader: reader}, size)
	return errors.Join(copyErr, file.Close())
}

// 延后创建链接，归档中的后续文件不能借助先前链接写到其他位置。
func extract(ctx context.Context, archive, directory, prefix string) (retErr error) {
	if err := os.Mkdir(directory, 0755); err != nil {
		return err
	}
	return extractInto(ctx, archive, directory, prefix)
}

// extractInto 只供同一私有准备目录组合 wheel；O_EXCL 阻止覆盖已有解释器和其他包文件。
func extractInto(ctx context.Context, archive, directory, prefix string) (retErr error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()
	u := unpacker{ctx: ctx, root: root, seen: map[string]bool{}, links: map[string]string{}}
	nameInRoot := func(name string) (string, error) {
		name = strings.TrimSuffix(name, "/")
		if prefix == "." {
			if name == "." {
				return "", nil
			}
			// Dugite 归档直接从 ./bin 等路径开始；只移除一个显式 ./，其余仍严格校验。
			name = strings.TrimPrefix(name, "./")
			if !safePath(name) {
				return "", errors.New("归档根路径无效")
			}
			return name, nil
		}
		if name == prefix {
			return "", nil
		}
		if !strings.HasPrefix(name, prefix+"/") {
			return "", errors.New("归档根目录与固定清单不符")
		}
		return strings.TrimPrefix(name, prefix+"/"), nil
	}
	if strings.HasSuffix(archive, ".zip") || strings.HasSuffix(archive, ".whl") {
		reader, err := zip.OpenReader(archive)
		if err != nil {
			return err
		}
		defer func() { retErr = errors.Join(retErr, reader.Close()) }()
		if len(reader.File) > 30000 {
			return errors.New("ZIP 条目数量超过限制")
		}
		for _, item := range reader.File {
			name, err := nameInRoot(item.Name)
			if err != nil {
				return err
			}
			if name == "" {
				continue
			}
			if item.UncompressedSize64 > 2<<30 || item.Mode()&os.ModeSymlink != 0 {
				return errors.New("ZIP 项目类型或大小无效")
			}
			stream, err := item.Open()
			if err != nil {
				return err
			}
			entryErr := u.entry(name, int64(item.UncompressedSize64), item.Mode(), stream, "")
			if err := errors.Join(entryErr, stream.Close()); err != nil {
				return err
			}
		}
	} else if strings.HasSuffix(archive, ".tar.gz") {
		file, err := os.Open(archive)
		if err != nil {
			return err
		}
		defer func() { retErr = errors.Join(retErr, file.Close()) }()
		compressed, err := gzip.NewReader(contextReader{ctx: ctx, reader: file})
		if err != nil {
			return err
		}
		defer func() { retErr = errors.Join(retErr, compressed.Close()) }()
		reader := tar.NewReader(compressed)
		count := 0
		for {
			item, err := reader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			count++
			if count > 30000 {
				return errors.New("TAR 条目数量超过限制")
			}
			name, err := nameInRoot(item.Name)
			if err != nil {
				return err
			}
			if name == "" {
				continue
			}
			if item.Typeflag != tar.TypeReg && item.Typeflag != tar.TypeDir && item.Typeflag != tar.TypeSymlink {
				return errors.New("TAR 包含特殊文件或硬链接")
			}
			if err := u.entry(name, item.Size, item.FileInfo().Mode(), reader, item.Linkname); err != nil {
				return err
			}
		}
		if _, err := io.Copy(io.Discard, io.LimitReader(compressed, 1<<20)); err != nil {
			return err
		}
	} else {
		return errors.New("仅支持 tar.gz 和 zip 运行时归档")
	}
	for name, link := range u.links {
		if err := root.Symlink(link, filepath.FromSlash(name)); err != nil {
			return err
		}
	}
	return nil
}
