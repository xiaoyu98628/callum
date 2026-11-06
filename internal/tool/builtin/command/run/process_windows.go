package run

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/xiaoyu98628/callum/internal/tool"
)

type process struct {
	handle, thread, job windows.Handle
	pid                 int
	finished            bool
	exit                processExit
}

func (p *process) processID() int { return p.pid }

func openTargetPath(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}

func startProcess(fixed *fixedCommand, stdin, stdout, stderr *os.File) (started *process, retErr error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if started == nil {
			retErr = errors.Join(retErr, windows.CloseHandle(job))
		}
	}()
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	// x/sys 的 Job 信息参数是 uintptr，显式固定内存，不能让栈移动或 GC 改变地址。
	var pinned runtime.Pinner
	pinned.Pin(&limits)
	defer pinned.Unpin()
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return nil, err
	}
	attributes, err := windows.NewProcThreadAttributeList(2)
	if err != nil {
		return nil, err
	}
	defer attributes.Delete()
	var handles []windows.Handle
	defer func() {
		for _, handle := range handles {
			retErr = errors.Join(retErr, windows.CloseHandle(handle))
		}
	}()
	for _, file := range []*os.File{stdin, stdout, stderr} {
		var handle windows.Handle
		if err := windows.DuplicateHandle(windows.CurrentProcess(), windows.Handle(file.Fd()), windows.CurrentProcess(), &handle, 0, true, windows.DUPLICATE_SAME_ACCESS); err != nil {
			return nil, err
		}
		handles = append(handles, handle)
	}
	if err := attributes.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), uintptr(len(handles))*unsafe.Sizeof(handles[0])); err != nil {
		return nil, err
	}
	// Windows 10+ 的创建时 Job 关联消除“启动后再加入”的运行窗口。
	// 属性值来自 Windows SDK winbase.h / ProcThreadAttributeJobList（13）。
	const procThreadAttributeJobList = 0x0002000d
	jobs := []windows.Handle{job}
	if err := attributes.Update(procThreadAttributeJobList, unsafe.Pointer(&jobs[0]), unsafe.Sizeof(jobs[0])); err != nil {
		return nil, err
	}
	application, err := windows.UTF16PtrFromString(fixed.executable.actual)
	if err != nil {
		return nil, err
	}
	line, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(fixed.command))
	if err != nil {
		return nil, err
	}
	cwd, err := windows.UTF16PtrFromString(fixed.directory.actual)
	if err != nil {
		return nil, err
	}
	environment := utf16.Encode([]rune(strings.Join(fixed.environment, "\x00") + "\x00\x00"))
	startup := windows.StartupInfoEx{ProcThreadAttributeList: attributes.List()}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	startup.Flags = windows.STARTF_USESTDHANDLES
	startup.StdInput, startup.StdOutput, startup.StdErr = handles[0], handles[1], handles[2]
	var native windows.ProcessInformation
	err = windows.CreateProcess(application, line, nil, nil, true, windows.CREATE_UNICODE_ENVIRONMENT|windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_NO_WINDOW, &environment[0], cwd, &startup.StartupInfo, &native)
	runtime.KeepAlive(environment)
	runtime.KeepAlive(handles)
	runtime.KeepAlive(jobs)
	if err != nil {
		return nil, err
	}
	return &process{handle: native.Process, thread: native.Thread, job: job, pid: int(native.ProcessId)}, nil
}

func (p *process) poll() (processExit, bool, error) {
	if p.finished {
		return p.exit, true, nil
	}
	state, err := windows.WaitForSingleObject(p.handle, 0)
	if err != nil {
		return processExit{}, false, err
	}
	if state == uint32(windows.WAIT_TIMEOUT) {
		return processExit{}, false, nil
	}
	if state != windows.WAIT_OBJECT_0 {
		return processExit{}, false, tool.ErrUnavailable
	}
	var code uint32
	if err := windows.GetExitCodeProcess(p.handle, &code); err != nil {
		return processExit{}, false, err
	}
	p.finished, p.exit.code = true, int(code)
	return p.exit, true, nil
}

func (p *process) stop() error { return windows.TerminateJobObject(p.job, 1) }

func (p *process) close() error {
	err := p.stop()
	// Job 的终止请求不等于全部成员已经退出，读取原生计数后再报告清理结果。
	deadline := time.Now().Add(cleanupTimeout)
	for {
		empty, queryErr := jobEmpty(p.job)
		if queryErr != nil {
			err = errors.Join(err, queryErr)
			break
		}
		if empty {
			break
		}
		if time.Now().After(deadline) {
			err = errors.Join(err, tool.ErrUnavailable)
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.Join(err, windows.CloseHandle(p.thread), windows.CloseHandle(p.handle), windows.CloseHandle(p.job))
}

func jobEmpty(job windows.Handle) (bool, error) {
	accounting := struct {
		TotalUser, TotalKernel, PeriodUser, PeriodKernel                 int64
		PageFaults, TotalProcesses, ActiveProcesses, TerminatedProcesses uint32
	}{}
	var pinned runtime.Pinner
	pinned.Pin(&accounting)
	defer pinned.Unpin()
	err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil)
	return accounting.ActiveProcesses == 0, err
}

func fileIdentity(file *os.File, _ os.FileInfo) (string, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return "", errors.Join(tool.ErrUnavailable, err)
	}
	return fmt.Sprintf("%d:%d:%d", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow), nil
}
