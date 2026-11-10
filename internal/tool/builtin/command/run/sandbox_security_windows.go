package run

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/xiaoyu98628/callum/internal/tool"
)

const sandboxFileAllAccess = 0x001f01ff
const sandboxDeleteChild = 0x40

var (
	sandboxUserenv            = windows.NewLazySystemDLL("userenv.dll")
	createAppContainerProfile = sandboxUserenv.NewProc("CreateAppContainerProfile")
	deleteAppContainerProfile = sandboxUserenv.NewProc("DeleteAppContainerProfile")
	deriveAppContainerSID     = sandboxUserenv.NewProc("DeriveAppContainerSidFromAppContainerName")
	createRestrictedToken     = windows.NewLazySystemDLL("advapi32.dll").NewProc("CreateRestrictedToken")
)

// securityCapabilities 的布局同时适用于 Windows x86 和 x64。
type securityCapabilities struct {
	SID          *windows.SID
	Capabilities *windows.SIDAndAttributes
	Count        uint32
	Reserved     uint32
}

func sandboxHRESULT(result uintptr) error {
	if int32(result) >= 0 {
		return nil
	}
	if uint32(result)>>16&0x1fff == 7 {
		return windows.Errno(uint32(result) & 0xffff)
	}
	return fmt.Errorf("Windows 沙箱 API 返回 HRESULT 0x%08x", uint32(result))
}

func sandboxSID(name string) (*windows.SID, error) {
	wide, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	var sid *windows.SID
	result, _, _ := deriveAppContainerSID.Call(uintptr(unsafe.Pointer(wide)), uintptr(unsafe.Pointer(&sid)))
	if err := sandboxHRESULT(result); err != nil {
		return nil, err
	}
	copy, copyErr := sid.Copy()
	return copy, errors.Join(copyErr, windows.FreeSid(sid))
}

func newSandboxProfile(name string, expected *windows.SID) (bool, error) {
	wide, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return false, err
	}
	var sid *windows.SID
	result, _, _ := createAppContainerProfile.Call(uintptr(unsafe.Pointer(wide)), uintptr(unsafe.Pointer(wide)), uintptr(unsafe.Pointer(wide)), 0, 0, uintptr(unsafe.Pointer(&sid)))
	if err := sandboxHRESULT(result); err != nil {
		return false, err
	}
	var identityErr error
	if !sid.Equals(expected) {
		identityErr = tool.ErrDenied
	}
	return true, errors.Join(identityErr, windows.FreeSid(sid))
}

func removeSandboxProfile(name string) error {
	wide, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	result, _, _ := deleteAppContainerProfile.Call(uintptr(unsafe.Pointer(wide)))
	err = sandboxHRESULT(result)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) || errors.Is(err, windows.ERROR_NOT_FOUND) {
		return nil
	}
	return err
}

func sandboxRestrictedToken(sid *windows.SID) (windows.Token, error) {
	var original, restricted windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_DUPLICATE|windows.TOKEN_QUERY|windows.TOKEN_ASSIGN_PRIMARY, &original); err != nil {
		return 0, err
	}
	entry := windows.SIDAndAttributes{Sid: sid}
	var pinned runtime.Pinner
	pinned.Pin(sid)
	pinned.Pin(&entry)
	defer pinned.Unpin()
	// WRITE_RESTRICTED 让任意写入还必须通过本次唯一 SID 的访问检查。
	// 不把 Everyone 或登录用户加入限制 SID，避免继承其他执行的可写范围。
	result, _, callErr := createRestrictedToken.Call(uintptr(original), 0x1|0x8, 0, 0, 0, 0, 1, uintptr(unsafe.Pointer(&entry)), uintptr(unsafe.Pointer(&restricted)))
	runtime.KeepAlive(sid)
	err := original.Close()
	if result == 0 {
		return 0, errors.Join(callErr, err)
	}
	if err != nil {
		return 0, errors.Join(err, restricted.Close())
	}
	return restricted, nil
}

func verifySandboxProcess(process windows.Handle, expected *windows.SID) (retErr error) {
	var token windows.Token
	if err := windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, token.Close()) }()
	var appContainer, returned uint32
	// TOKEN_INFORMATION_CLASS 的 TokenIsAppContainer 与 TokenAppContainerSid 分别为 29、31。
	if err := windows.GetTokenInformation(token, 29, (*byte)(unsafe.Pointer(&appContainer)), 4, &returned); err != nil {
		return err
	}
	if appContainer != 1 {
		return tool.ErrDenied
	}
	err := windows.GetTokenInformation(token, 31, nil, 0, &returned)
	if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) || returned < uint32(unsafe.Sizeof(uintptr(0))) || returned > 65536 {
		return errors.Join(tool.ErrDenied, err)
	}
	buffer := make([]byte, returned)
	if err := windows.GetTokenInformation(token, 31, &buffer[0], uint32(len(buffer)), &returned); err != nil {
		return err
	}
	sid := *(**windows.SID)(unsafe.Pointer(&buffer[0]))
	if sid == nil || !sid.Equals(expected) {
		return tool.ErrDenied
	}
	runtime.KeepAlive(buffer)
	restricted, err := token.IsRestricted()
	if err != nil {
		return err
	}
	if !restricted {
		return tool.ErrDenied
	}
	err = windows.GetTokenInformation(token, windows.TokenRestrictedSids, nil, 0, &returned)
	if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) || returned < uint32(unsafe.Sizeof(windows.Tokengroups{})) || returned > 65536 {
		return errors.Join(tool.ErrDenied, err)
	}
	buffer = make([]byte, returned)
	if err := windows.GetTokenInformation(token, windows.TokenRestrictedSids, &buffer[0], uint32(len(buffer)), &returned); err != nil {
		return err
	}
	groups := (*windows.Tokengroups)(unsafe.Pointer(&buffer[0]))
	if groups.GroupCount != 1 || !groups.Groups[0].Sid.Equals(expected) {
		return tool.ErrDenied
	}
	runtime.KeepAlive(buffer)
	return nil
}

func openSandboxACLPath(path string, directory, readOnly bool) (*os.File, error) {
	wide, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	// MAXIMUM_ALLOWED 阻止 SetSecurityInfo 自动向已有子项传播权限。
	// 逐个固定对象更新，才能跳过重解析点并保留每个对象的原有 ACL。
	access := uint32(windows.READ_CONTROL | windows.WRITE_DAC)
	if directory {
		access = windows.MAXIMUM_ALLOWED
	}
	if readOnly {
		access = windows.READ_CONTROL | windows.FILE_READ_ATTRIBUTES
	}
	handle, err := windows.CreateFile(wide, access, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}

func sandboxPathIdentity(path string) (identity string, retErr error) {
	file, err := openTargetPath(path)
	if err != nil {
		return "", err
	}
	defer func() { retErr = errors.Join(retErr, file.Close()) }()
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return "", err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return "", tool.ErrDenied
	}
	return fileIdentity(file, nil)
}

func updateSandboxACL(entry sandboxACLEntry, sid *windows.SID, remove bool) (retErr error) {
	file, err := openSandboxACLPath(entry.Path, entry.Directory, remove)
	if remove && (errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND)) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, file.Close()) }()
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return err
	}
	if !remove && info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return tool.ErrConflict
	}
	directory := info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0
	if !remove && directory != entry.Directory {
		return tool.ErrConflict
	}
	identity, err := fileIdentity(file, nil)
	if err != nil {
		return err
	}
	if !remove && entry.Identity != "" && identity != entry.Identity {
		return tool.ErrConflict
	}
	sd, err := windows.GetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if acl == nil {
		if remove {
			return nil
		}
		return tool.ErrDenied
	}
	if remove {
		filtered, changed, err := removeSandboxACEs(acl, sid)
		if err != nil || !changed {
			return err
		}
		access := uint32(windows.READ_CONTROL | windows.WRITE_DAC)
		if directory {
			access = windows.MAXIMUM_ALLOWED
		}
		proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("ReOpenFile")
		value, _, reopenErr := proc.Call(file.Fd(), uintptr(access), windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT)
		handle := windows.Handle(value)
		if handle == windows.InvalidHandle {
			return reopenErr
		}
		defer func() { retErr = errors.Join(retErr, windows.CloseHandle(handle)) }()
		err = windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, (*windows.ACL)(unsafe.Pointer(&filtered[0])), nil)
		runtime.KeepAlive(filtered)
		return err
	}
	var pinned runtime.Pinner
	pinned.Pin(sid)
	defer pinned.Unpin()
	trustee := windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_UNKNOWN, TrusteeValue: windows.TrusteeValueFromSID(sid)}
	inheritance := uint32(0)
	if entry.Directory {
		inheritance = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
	}
	mode := windows.ACCESS_MODE(windows.GRANT_ACCESS)
	if entry.Deny {
		mode = windows.DENY_ACCESS
	}
	entries := []windows.EXPLICIT_ACCESS{{AccessPermissions: windows.ACCESS_MASK(entry.Mask), AccessMode: mode, Inheritance: inheritance, Trustee: trustee}}
	if entry.Directory && !entry.Deny && entry.Mask&windows.FILE_WRITE_DATA != 0 {
		entries = append(entries, windows.EXPLICIT_ACCESS{AccessPermissions: windows.DELETE, AccessMode: windows.GRANT_ACCESS, Inheritance: inheritance | windows.INHERIT_ONLY_ACE, Trustee: trustee})
	}
	if entry.Deny && entry.Mask != sandboxFileAllAccess {
		entries = append(entries, windows.EXPLICIT_ACCESS{AccessPermissions: windows.FILE_GENERIC_READ | windows.FILE_GENERIC_EXECUTE, AccessMode: windows.GRANT_ACCESS, Inheritance: inheritance, Trustee: trustee})
	}
	updated, err := windows.ACLFromEntries(entries, acl)
	if err != nil {
		return err
	}
	return windows.SetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, updated, nil)
}

// 已有系统安装目录可通过 All Application Packages 提供读取，不修改这些目录的 ACL。
func ambientWindowsRead(path string) bool {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return false
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return false
	}
	sid, err := windows.StringToSid("S-1-15-2-1")
	if err != nil {
		return false
	}
	var pinned runtime.Pinner
	pinned.Pin(sid)
	defer pinned.Unpin()
	trustee := windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_WELL_KNOWN_GROUP, TrusteeValue: windows.TrusteeValueFromSID(sid)}
	var mask uint32
	proc := windows.NewLazySystemDLL("advapi32.dll").NewProc("GetEffectiveRightsFromAclW")
	result, _, _ := proc.Call(uintptr(unsafe.Pointer(acl)), uintptr(unsafe.Pointer(&trustee)), uintptr(unsafe.Pointer(&mask)))
	runtime.KeepAlive(sd)
	if result != 0 {
		return false
	}
	want := uint32(windows.FILE_GENERIC_READ | windows.FILE_GENERIC_EXECUTE)
	return mask&want == want || mask&windows.GENERIC_ALL != 0
}

// 回收只移除本次 SID 的 ACE，保留其他执行及用户在运行期间新增的权限。
func removeSandboxACEs(acl *windows.ACL, sid *windows.SID) ([]byte, bool, error) {
	header := unsafe.Slice((*byte)(unsafe.Pointer(acl)), 8)
	size := int(binary.LittleEndian.Uint16(header[2:4]))
	if size < 8 {
		return nil, false, tool.ErrInvalid
	}
	result := append([]byte(nil), header...)
	var count uint16
	changed := false
	for index := uint32(0); index < uint32(acl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, index, &ace); err != nil {
			return nil, false, err
		}
		if ace.Header.AceSize < 4 || int(ace.Header.AceSize) > size {
			return nil, false, tool.ErrInvalid
		}
		if (ace.Header.AceType == windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE) && ace.Header.AceSize >= 16 {
			aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
			if aceSID.IsValid() && aceSID.Equals(sid) {
				changed = true
				continue
			}
		}
		result = append(result, unsafe.Slice((*byte)(unsafe.Pointer(ace)), int(ace.Header.AceSize))...)
		count++
	}
	binary.LittleEndian.PutUint16(result[2:4], uint16(len(result)))
	binary.LittleEndian.PutUint16(result[4:6], count)
	return result, changed, nil
}
