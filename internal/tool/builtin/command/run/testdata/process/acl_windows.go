package main

import "golang.org/x/sys/windows"

// 仅用于隔离测试，尝试通过改变 ACL 绕过执行边界。
func relaxACL(path string) error {
	sd, err := windows.SecurityDescriptorFromString("D:(A;;FA;;;WD)")
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}
