package patch

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/windows"

	"github.com/xiaoyu98628/callum/internal/tool"
)

func patchFileReady(file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return tool.ErrUnavailable
	}
	var native windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &native) != nil {
		return tool.ErrUnavailable
	}
	if !info.Mode().IsRegular() || native.NumberOfLinks != 1 || native.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return tool.ErrDenied
	}
	return nil
}

// 新文件和恢复副本使用当前进程用户的受保护 DACL，不继承父目录的宽松权限。
// 已有文件通过原句柄更新，保留其原有 DACL、所有者及其他权限。
func protectPatchFile(file *os.File) (retErr error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	// os.Root.OpenFile 的数据句柄未请求 WRITE_DAC。按原句柄重新打开相同
	// 对象以修改新建对象的权限，不用路径重开，避免路径替换导致误改权限。
	library, err := windows.LoadLibraryEx("kernel32.dll", 0, windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, windows.FreeLibrary(library)) }()
	address, err := windows.GetProcAddress(library, "ReOpenFile")
	if err != nil {
		return err
	}
	value, _, err := syscall.SyscallN(address, file.Fd(), windows.WRITE_DAC|windows.READ_CONTROL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_FLAG_BACKUP_SEMANTICS)
	handle := windows.Handle(value)
	if handle == windows.InvalidHandle {
		return err
	}
	defer func() { retErr = errors.Join(retErr, windows.CloseHandle(handle)) }()
	return windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}
func protectPatchDirectory(file *os.File) error { return protectPatchFile(file) }
