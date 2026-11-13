package attachment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/xiaoyu98628/callum/internal/model"
)

// RunFiles 由服务端创建并绑定一次执行，不能由模型或客户端指定。
type RunFiles struct {
	ConversationID string
	Inputs         string
	Work           string
	Artifacts      []model.Block
	directory      string
	bytes          int64
	files          map[string]string
}

// Access 保管命令读取附件所需的工作副本。实例锁覆盖恢复，运行调度器结束后由 Runtime 关闭。
type Access struct {
	store              *Store
	resolve            func(context.Context, string, string) (model.Block, error)
	markers, temporary string
	mu                 sync.Mutex
	runs               map[string]*RunFiles
	closed             bool
}

// OpenAccess 恢复上次异常退出的自有工作目录，不接触用户原图或文档。
func OpenAccess(ctx context.Context, dataDir string, store *Store, resolve func(context.Context, string, string) (model.Block, error)) (*Access, error) {
	if store == nil || resolve == nil {
		return nil, errors.New("附件访问配置无效")
	}
	markers := filepath.Join(dataDir, ".attachment-runs")
	if err := privateDirectory(markers); err != nil {
		return nil, err
	}
	temporary, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		return nil, err
	}
	a := &Access{store: store, resolve: resolve, markers: markers, temporary: temporary, runs: make(map[string]*RunFiles)}
	if err := a.recover(ctx); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *Access) marker(runID string) string { return filepath.Join(a.markers, runID+".json") }

// Files 仅向装配好的工具调度器返回当前执行的路径，不接受模型指定根目录。
func (a *Access) Files(conversationID, runID string) (RunFiles, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	run := a.runs[runID]
	if a.closed || run == nil || run.ConversationID != conversationID {
		return RunFiles{}, false
	}
	copy := *run
	copy.Artifacts = append([]model.Block(nil), run.Artifacts...)
	return copy, true
}

func (a *Access) recover(ctx context.Context) (retErr error) {
	file, err := os.Open(a.markers)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, file.Close()) }()
	for {
		entries, err := file.ReadDir(64)
		if err != nil && err != io.EOF {
			return err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			runID := strings.TrimSuffix(entry.Name(), ".json")
			if !validID(runID) || entry.Name() != runID+".json" || !entry.Type().IsRegular() {
				return errors.New("附件工作目录恢复标记无效")
			}
			marker, err := os.OpenFile(a.marker(runID), readFlags, 0)
			if err != nil {
				return err
			}
			info, statErr := marker.Stat()
			if statErr != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
				return errors.Join(errors.New("附件恢复标记类型或大小无效"), statErr, marker.Close())
			}
			data, readErr := io.ReadAll(io.LimitReader(marker, 4097))
			if err := errors.Join(readErr, marker.Close()); err != nil {
				return err
			}
			if len(data) > 4096 {
				return errors.New("附件工作目录恢复记录不可读")
			}
			var directory string
			if json.Unmarshal(data, &directory) != nil || filepath.Dir(directory) != a.temporary || !strings.HasPrefix(filepath.Base(directory), "callum-attachments-"+runID+"-") {
				return errors.New("附件工作目录恢复路径无效")
			}
			if err := os.RemoveAll(directory); err != nil {
				return err
			}
			if err := os.Remove(a.marker(runID)); err != nil {
				return err
			}
		}
		if err == io.EOF {
			return nil
		}
	}
}

// Begin 创建独立私有目录。副本直到对应执行的所有工具结束后才回收。
func (a *Access) Begin(ctx context.Context, conversationID, runID string) (_ RunFiles, retErr error) {
	if !validID(conversationID) || !validID(runID) {
		return RunFiles{}, model.ErrAttachment
	}
	if err := ctx.Err(); err != nil {
		return RunFiles{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.runs[runID] != nil || len(a.runs) >= 64 {
		return RunFiles{}, model.ErrAttachment
	}
	directory, err := os.MkdirTemp(a.temporary, "callum-attachments-"+runID+"-")
	if err != nil {
		return RunFiles{}, err
	}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, os.RemoveAll(directory))
		}
	}()
	value := &RunFiles{ConversationID: conversationID, Inputs: filepath.Join(directory, "inputs"), Work: filepath.Join(directory, "work"), directory: directory, files: make(map[string]string)}
	for _, path := range []string{value.Inputs, value.Work} {
		if err := os.Mkdir(path, 0700); err != nil {
			return RunFiles{}, err
		}
	}
	data, err := json.Marshal(directory)
	if err != nil {
		return RunFiles{}, err
	}
	marker, err := os.OpenFile(a.marker(runID), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return RunFiles{}, err
	}
	_, writeErr := marker.Write(data)
	if err := errors.Join(writeErr, marker.Sync(), marker.Close()); err != nil {
		return RunFiles{}, errors.Join(err, os.Remove(a.marker(runID)))
	}
	// 文件内容先同步，恢复标记再固定，异常退出时仅按本实例记录回收。
	if runtime.GOOS != "windows" {
		parent, err := os.Open(a.markers)
		if err != nil {
			return RunFiles{}, err
		}
		if err := errors.Join(parent.Sync(), parent.Close()); err != nil {
			return RunFiles{}, err
		}
	}
	a.runs[runID] = value
	return *value, nil
}

// Metadata 只从当前消息分支解析引用，并固定原件身份；调用者负责关闭 File。
func (a *Access) Metadata(ctx context.Context, conversationID, runID, ref string) (model.Block, *File, error) {
	a.mu.Lock()
	run := a.runs[runID]
	allowed := !a.closed && run != nil && run.ConversationID == conversationID
	a.mu.Unlock()
	if !allowed || !validReference(ref, conversationID) {
		return model.Block{}, nil, model.ErrAttachment
	}
	block, err := a.resolve(ctx, conversationID, ref)
	if err != nil || block.Type != "file" || block.AttachmentRef != ref {
		return model.Block{}, nil, errors.Join(model.ErrAttachment, err)
	}
	file, err := a.store.Pin(ctx, ref)
	return block, file, err
}

// Export 在审批完成后校验原件哈希并生成独立副本，不使用硬链接。
func (a *Access) Export(ctx context.Context, conversationID, runID string, block model.Block, file *File) (string, string, error) {
	if _, err := a.resolve(ctx, conversationID, block.AttachmentRef); err != nil {
		return "", "", errors.Join(model.ErrAttachment, err)
	}
	data, err := file.Read(ctx)
	if err != nil {
		return "", "", err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	run := a.runs[runID]
	if a.closed || run == nil || run.ConversationID != conversationID {
		return "", "", model.ErrAttachment
	}
	if path := run.files[block.AttachmentRef]; path != "" {
		cached, err := os.OpenFile(path, readFlags, 0)
		if err != nil {
			return "", "", err
		}
		info, statErr := cached.Stat()
		if statErr != nil || !info.Mode().IsRegular() || info.Size() != int64(len(data)) {
			return "", "", errors.Join(model.ErrAttachment, statErr, cached.Close())
		}
		current, readErr := readBytes(ctx, cached)
		err = errors.Join(readErr, cached.Close())
		if err != nil || !bytes.Equal(current, data) {
			return "", "", model.ErrAttachment
		}
		return path, run.Work, nil
	}
	if run.bytes+int64(len(data)) > 128<<20 || len(run.files) >= 1024 {
		return "", "", model.ErrAttachment
	}
	name := block.Name
	if name == "" {
		name = filepath.Base(block.AttachmentRef)
	}
	if !ValidName(name) {
		return "", "", model.ErrAttachment
	}
	hash := sha256.Sum256([]byte(block.AttachmentRef))
	directory := filepath.Join(run.Inputs, hex.EncodeToString(hash[:]))
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", "", err
	}
	path := filepath.Join(directory, name)
	output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", "", err
	}
	_, writeErr := output.Write(data)
	if err := errors.Join(writeErr, output.Close()); err != nil {
		return "", "", errors.Join(err, os.Remove(path))
	}
	run.bytes += int64(len(data))
	run.files[block.AttachmentRef] = path
	return path, run.Work, ctx.Err()
}

// End 可重复调用，正常、取消与失败共用清理。失败保留恢复标记供重启处理。
func (a *Access) End(runID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	run := a.runs[runID]
	if run == nil {
		return nil
	}
	if err := os.RemoveAll(run.directory); err != nil {
		return err
	}
	if err := os.Remove(a.marker(runID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	delete(a.runs, runID)
	return nil
}

func (a *Access) Close() error {
	a.mu.Lock()
	a.closed = true
	ids := make([]string, 0, len(a.runs))
	for id := range a.runs {
		ids = append(ids, id)
	}
	a.mu.Unlock()
	var err error
	for _, id := range ids {
		err = errors.Join(err, a.End(id))
	}
	return err
}
