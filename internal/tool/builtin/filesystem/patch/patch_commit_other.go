//go:build !darwin && !linux && !windows

package patch

import (
	"github.com/xiaoyu98628/callum/internal/tool"
	"os"
)

// 未具备固定句柄及权限保障的平台拒绝写入，读取工具仍可使用。
func patchFileReady(*os.File) error        { return tool.ErrUnavailable }
func protectPatchFile(*os.File) error      { return tool.ErrUnavailable }
func protectPatchDirectory(*os.File) error { return tool.ErrUnavailable }
