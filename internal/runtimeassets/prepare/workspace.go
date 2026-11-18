package prepare

import (
	"encoding/hex"
	"errors"
	"strings"

	"github.com/xiaoyu98628/callum/internal/runtimeassets"
)

// WorkspaceSource 记录用户指定技能运行时的离线归档，不在准备或运行期间联网获取。
type WorkspaceSource struct {
	Runtime  runtimeassets.WorkspaceRuntime `json:"runtime"`
	Archives []WorkspaceArchive             `json:"archives"`
}

type WorkspaceArchive struct {
	Archive string `json:"archive"`
	SHA256  string `json:"sha256"`
}

func validateWorkspace(source *WorkspaceSource) error {
	if source == nil {
		return nil
	}
	r := source.Runtime
	if r.Version == "" || len(source.Archives) == 0 || len(source.Archives) > 32 {
		return errors.New("工作区依赖资源版本或归档数量无效")
	}
	for _, name := range []string{r.NodeModules, r.BinDirectory} {
		if !safePath(name) {
			return errors.New("工作区依赖入口路径无效")
		}
	}
	if !strings.HasPrefix(r.NodeModules, "node/") || r.BinDirectory != "native" {
		return errors.New("技能依赖必须位于 node 与 native 目录")
	}
	seen := map[string]bool{}
	for _, archive := range source.Archives {
		_, err := hex.DecodeString(archive.SHA256)
		if !simpleName(archive.Archive) || !strings.HasSuffix(archive.Archive, ".tar.gz") || seen[archive.Archive] || err != nil || len(archive.SHA256) != 64 {
			return errors.New("工作区依赖归档或校验值无效")
		}
		seen[archive.Archive] = true
	}
	return nil
}

func sameWorkspace(manifest *runtimeassets.WorkspaceRuntime, source *WorkspaceSource) bool {
	if source == nil || manifest == nil {
		return source == nil && manifest == nil
	}
	return *manifest == source.Runtime
}
