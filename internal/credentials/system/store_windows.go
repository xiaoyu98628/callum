//go:build windows

package system

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"unsafe"

	"github.com/xiaoyu98628/callum/internal/credentials"
	"golang.org/x/sys/windows"
)

type genericCredential struct {
	Flags, Type             uint32
	TargetName, Comment     *uint16
	LastWritten             windows.Filetime
	CredentialBlobSize      uint32
	CredentialBlob          *byte
	Persist, AttributeCount uint32
	Attributes              unsafe.Pointer
	TargetAlias, UserName   *uint16
}

// 每次操作独占加载系统 DLL，归还加载引用，不留下隐藏的全局句柄。
func credentialDLL() (*windows.DLL, error) {
	directory, err := windows.GetSystemDirectory()
	if err != nil {
		return nil, credentials.ErrUnavailable
	}
	library, err := windows.LoadDLL(filepath.Join(directory, "advapi32.dll"))
	if err != nil {
		return nil, credentials.ErrUnavailable
	}
	return library, nil
}

func (Store) Save(ctx context.Context, name string, key []byte) (retErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validName(name) || len(key) != 32 {
		return credentials.ErrInvalid
	}
	library, err := credentialDLL()
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, library.Release()) }()
	write, err := library.FindProc("CredWriteW")
	if err != nil {
		return credentials.ErrUnavailable
	}
	target, err := windows.UTF16PtrFromString(service + ":" + name)
	if err != nil {
		return credentials.ErrInvalid
	}
	value := genericCredential{Type: 1, TargetName: target, CredentialBlobSize: uint32(len(key)), CredentialBlob: &key[0], Persist: 2}
	ok, _, _ := write.Call(uintptr(unsafe.Pointer(&value)), 0)
	runtime.KeepAlive(key)
	if ok == 0 {
		return credentials.ErrUnavailable
	}
	return ctx.Err()
}

func (Store) Load(ctx context.Context, name string) (key []byte, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validName(name) {
		return nil, credentials.ErrInvalid
	}
	library, err := credentialDLL()
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, library.Release()) }()
	read, err := library.FindProc("CredReadW")
	if err != nil {
		return nil, credentials.ErrUnavailable
	}
	free, err := library.FindProc("CredFree")
	if err != nil {
		return nil, credentials.ErrUnavailable
	}
	target, err := windows.UTF16PtrFromString(service + ":" + name)
	if err != nil {
		return nil, credentials.ErrInvalid
	}
	var value *genericCredential
	ok, _, callErr := read.Call(uintptr(unsafe.Pointer(target)), 1, 0, uintptr(unsafe.Pointer(&value)))
	if ok == 0 {
		if errors.Is(callErr, windows.ERROR_NOT_FOUND) {
			return nil, credentials.ErrNotFound
		}
		return nil, credentials.ErrUnavailable
	}
	defer free.Call(uintptr(unsafe.Pointer(value)))
	if value == nil || value.CredentialBlobSize != 32 || value.CredentialBlob == nil {
		return nil, credentials.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]byte(nil), unsafe.Slice(value.CredentialBlob, 32)...), nil
}
