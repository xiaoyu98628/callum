package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/xiaoyu98628/callum/internal/tool"
)

func ensureSandboxJournalDirectory(path string) error {
	created := false
	if err := os.Mkdir(path, 0700); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
	} else {
		created = true
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return tool.ErrDenied
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	if created {
		return secureNewSandboxDirectory(path)
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	if !owner.Equals(user.User.Sid) {
		return tool.ErrDenied
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return errors.Join(tool.ErrDenied, err)
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return err
	}
	for index := uint32(0); index < uint32(acl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, index, &ace); err != nil {
			return err
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		// 私有恢复目录不接受无法验证受托人的回调或对象专用授权条目。
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return tool.ErrDenied
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !sid.Equals(user.User.Sid) && !sid.Equals(system) {
			return tool.ErrDenied
		}
	}
	return nil
}

func secureNewSandboxDirectory(path string) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, user.User.Sid, nil, acl, nil)
}

func writeSandboxJournal(path string, journal sandboxJournal) (retErr error) {
	data, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	if len(data) > maxSandboxJournalBytes {
		return tool.ErrLimit
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() {
		retErr = errors.Join(retErr, file.Close())
		if retErr != nil {
			retErr = errors.Join(retErr, os.Remove(path))
		}
	}()
	if _, err := file.Write(data); err != nil {
		return err
	}
	return file.Sync()
}

func withSandboxACLLock(operation func() error) (retErr error) {
	// Windows Mutex 归操作系统线程所有，持有期间不能让 goroutine 切换线程。
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(`Global\CallumSandboxACL.` + user.User.Sid.String())
	if err != nil {
		return err
	}
	mutex, err := windows.CreateMutex(nil, false, name)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return err
	}
	defer func() { retErr = errors.Join(retErr, windows.CloseHandle(mutex)) }()
	state, err := windows.WaitForSingleObject(mutex, 5000)
	if err != nil {
		return err
	}
	if state != windows.WAIT_OBJECT_0 && state != windows.WAIT_ABANDONED {
		return tool.ErrUnavailable
	}
	defer func() { retErr = errors.Join(retErr, windows.ReleaseMutex(mutex)) }()
	return operation()
}

func sandboxJobName(profile string) string { return `Global\CallumSandbox.` + profile }

// stopSandboxJob 在回收权限前确认旧进程树已经结束，避免清理期间继续执行。
func stopSandboxJob(profile string) (retErr error) {
	name, err := windows.UTF16PtrFromString(sandboxJobName(profile))
	if err != nil {
		return err
	}
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("OpenJobObjectW")
	result, _, callErr := proc.Call(0x4|0x8, 0, uintptr(unsafe.Pointer(name)))
	if result == 0 {
		if errors.Is(callErr, windows.ERROR_FILE_NOT_FOUND) {
			return nil
		}
		return callErr
	}
	job := windows.Handle(result)
	defer func() { retErr = errors.Join(retErr, windows.CloseHandle(job)) }()
	if err := windows.TerminateJobObject(job, 1); err != nil {
		return err
	}
	for deadline := time.Now().Add(cleanupTimeout); ; {
		empty, err := jobEmpty(job)
		if err != nil {
			return err
		}
		if empty {
			return nil
		}
		if time.Now().After(deadline) {
			return tool.ErrUnavailable
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (s *sandboxState) cleanup() error {
	var err error
	if s.token != 0 {
		err = s.token.Close()
		s.token = 0
	}
	if s.active {
		if stopErr := stopSandboxJob(s.journal.Name); stopErr != nil {
			return errors.Join(err, stopErr)
		}
		if s.profileOwned {
			err = errors.Join(err, removeSandboxProfile(s.journal.Name))
			if err != nil {
				return err
			}
			s.profileOwned = false
		}
		if s.aclTouched {
			err = errors.Join(err, withSandboxACLLock(func() error {
				var cleanupErr error
				for _, entry := range s.journal.Entries {
					cleanupErr = errors.Join(cleanupErr, updateSandboxACL(entry, s.sid, true))
				}
				// 新建及项目内改名的对象可能继承 SID；进程树结束后统一回收。
				roots := append([]sandboxRoot(nil), s.journal.Roots...)
				roots = append(roots, sandboxRoot{Path: s.journal.Temporary}, sandboxRoot{Path: s.journal.ProtectedDirectory})
				for _, root := range roots {
					if root.Ambient {
						continue
					}
					visited := 0
					walkErr := filepath.WalkDir(root.Path, func(path string, entry fs.DirEntry, walkErr error) error {
						if errors.Is(walkErr, os.ErrNotExist) {
							return nil
						}
						if walkErr != nil {
							return walkErr
						}
						visited++
						if visited > maxSandboxEntries {
							return tool.ErrLimit
						}
						return updateSandboxACL(sandboxACLEntry{Path: path, Directory: entry.IsDir()}, s.sid, true)
					})
					cleanupErr = errors.Join(cleanupErr, walkErr)
				}
				return cleanupErr
			}))
		}
		if err != nil {
			return fmt.Errorf("Windows 沙箱权限回收未完成，恢复记录保留于 %s: %w", s.journalPath, err)
		}
	}
	if s.journal.Temporary != "" {
		for _, entry := range s.journal.Entries {
			if !strings.EqualFold(entry.Path, s.journal.Temporary) {
				continue
			}
			identity, identityErr := sandboxPathIdentity(entry.Path)
			if identityErr != nil && !errors.Is(identityErr, os.ErrNotExist) {
				return identityErr
			}
			if identityErr == nil && identity != entry.Identity {
				return tool.ErrConflict
			}
		}
		err = errors.Join(err, os.RemoveAll(s.journal.Temporary))
	}
	if err != nil {
		return err
	}
	if s.active {
		if err := os.Remove(s.journalPath); err != nil {
			return err
		}
		s.active = false
	}
	return nil
}

// RecoverSandbox 在实例锁保护下回收中断执行的 Windows 沙箱，失败时阻止应用继续初始化。
func RecoverSandbox(ctx context.Context, dataDirectory string) error {
	directory := filepath.Join(dataDirectory, "command-sandboxes")
	if _, err := os.Lstat(directory); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	dataDirectory, err := filepath.EvalSymlinks(dataDirectory)
	if err != nil {
		return err
	}
	directory = filepath.Join(dataDirectory, "command-sandboxes")
	if err := ensureSandboxJournalDirectory(directory); err != nil {
		return err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), ".json") {
			return tool.ErrDenied
		}
		path := filepath.Join(directory, entry.Name())
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxSandboxJournalBytes+1))
		if err := errors.Join(readErr, file.Close()); err != nil {
			return err
		}
		if len(data) > maxSandboxJournalBytes {
			return tool.ErrLimit
		}
		var journal sandboxJournal
		decoder := json.NewDecoder(strings.NewReader(string(data)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&journal); err != nil {
			return err
		}
		if decoder.Decode(new(any)) != io.EOF {
			return tool.ErrInvalid
		}
		if journal.Version != sandboxJournalVersion || !strings.HasPrefix(journal.Name, "callum.command.") || entry.Name() != journal.Name+".json" || len(journal.Name) != len("callum.command.")+26 {
			return tool.ErrInvalid
		}
		if !strings.EqualFold(journal.ProtectedDirectory, dataDirectory) {
			return tool.ErrDenied
		}
		sid, err := sandboxSID(journal.Name)
		if err != nil {
			return err
		}
		if sid.String() != journal.SID {
			return tool.ErrDenied
		}
		if !filepath.IsAbs(journal.Temporary) || !strings.HasPrefix(filepath.Base(journal.Temporary), "callum-command-") || len(filepath.Base(journal.Temporary)) != len("callum-command-")+26 {
			return tool.ErrDenied
		}
		if len(journal.Entries) > maxSandboxEntries || len(journal.Roots) > 3 {
			return tool.ErrLimit
		}
		for _, root := range journal.Roots {
			if !filepath.IsAbs(root.Path) {
				return tool.ErrInvalid
			}
		}
		for _, aclEntry := range journal.Entries {
			if !filepath.IsAbs(aclEntry.Path) {
				return tool.ErrInvalid
			}
			allowed := withinSandboxPath(aclEntry.Path, dataDirectory) || withinSandboxPath(aclEntry.Path, journal.Temporary)
			for _, root := range journal.Roots {
				allowed = allowed || withinSandboxPath(aclEntry.Path, root.Path)
			}
			if !allowed {
				return tool.ErrDenied
			}
		}
		state := sandboxState{journal: journal, journalPath: path, sid: sid, active: true, profileOwned: true, aclTouched: true}
		if err := state.cleanup(); err != nil {
			return fmt.Errorf("回收 Windows 沙箱失败: %w", err)
		}
	}
	return nil
}

func withinSandboxPath(path, root string) bool {
	relative, err := filepath.Rel(strings.ToLower(root), strings.ToLower(path))
	return err == nil && filepath.IsLocal(relative)
}
