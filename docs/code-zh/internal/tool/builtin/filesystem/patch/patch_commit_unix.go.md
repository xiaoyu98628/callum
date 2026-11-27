```go
//go:build darwin || linux

package 补丁

import (
	错误判断包 "errors"
	操作系统包 "os"
	运行时包 "runtime"
	系统调用包 "syscall"
	不安全操作包 "unsafe"

	Unix包 "golang.org/x/sys/unix"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

// 通过原文件句柄写入保留所有者、权限及 ACL。硬链接会将修改传播到未批准
// 的其他路径，因此写入前及实际提交前均拒绝多链接文件。
func 补丁文件就绪(文件 *操作系统包.文件) error {
	信息, 错误 := 文件.状态信息()
	if 错误 != nil {
		return 工具包.不可用错误
	}
	状态信息, 成功 := 信息.系统().(*系统调用包.文件状态结构)
	if !成功 || !信息.模式().是否普通文件() || 状态信息.硬链接数量 != 1 || 信息.模式()&(操作系统包.模式设置用户标识位|操作系统包.模式设置组标识位|操作系统包.模式粘滞位) != 0 {
		return 工具包.已拒绝错误
	}
	if 运行时包.目标操作系统 == "linux" {
		// Linux 写入可能清除文件 能力集；不能静默改变已有执行权限。
		大小, 错误 := Unix包.读取扩展属性(int(文件.文件描述符()), "security.capability", nil)
		if 错误 == nil && 大小 > 0 {
			return 工具包.已拒绝错误
		}
		if 错误 != nil && !错误判断包.是否属于(错误, Unix包.无数据错误) && !错误判断包.是否属于(错误, Unix包.不支持错误) {
			return 工具包.不可用错误
		}
	}
	return nil
}
func 保护补丁文件(文件 *操作系统包.文件) error      { return 保护补丁模式(文件, 0600) }
func 保护补丁目录(文件 *操作系统包.文件) error { return 保护补丁模式(文件, 0700) }

func 保护补丁模式(文件 *操作系统包.文件, 模式 操作系统包.文件模式) error {
	if 运行时包.目标操作系统 != "darwin" {
		return 文件.修改权限(模式)
	}
	// macOS 的 修改权限 不删除继承的 ACL。通过固定句柄同时设置私有模式和
	// KAUTH_FILESEC_NOACL，避免恢复副本继承父目录的额外读取授权。
	// ABI 来自 macOS SDK 系统/kauth.处理器、系统/系统调用.处理器（fchmod_extended=283）；
	// Go 不提供该扩展的封装，保持纯 Go 构建，系统拒绝时不继续写入内容。
	安全 := struct {
		魔数             uint32
		所属对象, 组      [16]byte
		条目数量, 参数集 uint32
	}{魔数: 0x012cc16d, 条目数量: ^uint32(0)}
	连接, 错误 := 文件.系统调用连接()
	if 错误 != nil {
		return 错误
	}
	var 调用错误 系统调用包.系统错误码
	错误 = 连接.控制(func(文件描述符 uintptr) {
		const 无身份 = ^uint32(0) - 100 // KAUTH_UID_NONE、KAUTH_GID_NONE；并非 chown 的 -1。
		_, _, 调用错误 = 系统调用包.系统调用6(283, 文件描述符, uintptr(无身份), uintptr(无身份), uintptr(模式.权限()), uintptr(不安全操作包.指针(&安全)), 0)
	})
	运行时包.保留存活(&安全)
	if 错误 != nil {
		return 错误
	}
	if 调用错误 != 0 {
		return 调用错误
	}
	return nil
}
```
