//go:build darwin || linux

package patch

import (
	"errors"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/xiaoyu98628/callum/internal/tool"
)

// 通过原文件句柄写入保留所有者、权限及 ACL。硬链接会将修改传播到未批准
// 的其他路径，因此写入前及实际提交前均拒绝多链接文件。
func patchFileReady(file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return tool.ErrUnavailable
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || stat.Nlink != 1 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return tool.ErrDenied
	}
	if runtime.GOOS == "linux" {
		// Linux 写入可能清除文件 capabilities；不能静默改变已有执行权限。
		size, err := unix.Fgetxattr(int(file.Fd()), "security.capability", nil)
		if err == nil && size > 0 {
			return tool.ErrDenied
		}
		if err != nil && !errors.Is(err, unix.ENODATA) && !errors.Is(err, unix.ENOTSUP) {
			return tool.ErrUnavailable
		}
	}
	return nil
}
func protectPatchFile(file *os.File) error      { return protectPatchMode(file, 0600) }
func protectPatchDirectory(file *os.File) error { return protectPatchMode(file, 0700) }

func protectPatchMode(file *os.File, mode os.FileMode) error {
	if runtime.GOOS != "darwin" {
		return file.Chmod(mode)
	}
	// Darwin 的 chmod 不删除继承的 ACL。通过固定句柄同时设置私有模式和
	// KAUTH_FILESEC_NOACL，避免恢复副本继承父目录的额外读取授权。
	// ABI 来自 macOS SDK sys/kauth.h、sys/syscall.h（fchmod_extended=283）；
	// Go 不提供该扩展的封装，保持纯 Go 构建，系统拒绝时不继续写入内容。
	security := struct {
		Magic             uint32
		Owner, Group      [16]byte
		EntryCount, Flags uint32
	}{Magic: 0x012cc16d, EntryCount: ^uint32(0)}
	conn, err := file.SyscallConn()
	if err != nil {
		return err
	}
	var callErr syscall.Errno
	err = conn.Control(func(fd uintptr) {
		const noIdentity = ^uint32(0) - 100 // KAUTH_UID_NONE、KAUTH_GID_NONE；并非 chown 的 -1。
		_, _, callErr = syscall.Syscall6(283, fd, uintptr(noIdentity), uintptr(noIdentity), uintptr(mode.Perm()), uintptr(unsafe.Pointer(&security)), 0)
	})
	runtime.KeepAlive(&security)
	if err != nil {
		return err
	}
	if callErr != 0 {
		return callErr
	}
	return nil
}
