```go
//go:build windows

package 系统

import (
	上下文包 "context"
	错误判断包 "errors"
	文件路径包 "path/filepath"
	运行时包 "runtime"
	不安全操作包 "unsafe"

	凭据包 "github.com/xiaoyu98628/callum/internal/credentials"
	Windows包 "golang.org/x/sys/windows"
)

type 通用凭据 struct {
	参数集, 类型    uint32
	目标名称, 注释   *uint16
	最后已写入      Windows包.文件时间
	凭据二进制大对象大小 uint32
	凭据二进制大对象   *byte
	持久化, 属性数量  uint32
	属性集        不安全操作包.指针
	目标别名, 用户名称 *uint16
}

// 每次操作独占加载系统 动态链接库，归还加载引用，不留下隐藏的全局句柄。
func 凭据动态链接库() (*Windows包.动态链接库, error) {
	目录, 错误 := Windows包.获取系统目录()
	if 错误 != nil {
		return nil, 凭据包.不可用错误
	}
	库, 错误 := Windows包.加载动态链接库(文件路径包.合并(目录, "advapi32.dll"))
	if 错误 != nil {
		return nil, 凭据包.不可用错误
	}
	return 库, nil
}

func (存储) 保存(执行上下文 上下文包.上下文, 名称 string, 键 []byte) (返回错误 error) {
	if 错误 := 执行上下文.获取错误(); 错误 != nil {
		return 错误
	}
	if !有效名称(名称) || len(键) != 32 {
		return 凭据包.无效错误
	}
	库, 错误 := 凭据动态链接库()
	if 错误 != nil {
		return 错误
	}
	defer func() { 返回错误 = 错误判断包.合并(返回错误, 库.释放()) }()
	写入, 错误 := 库.查找进程("CredWriteW")
	if 错误 != nil {
		return 凭据包.不可用错误
	}
	目标, 错误 := Windows包.UTF16指针来源字符串(服务 + ":" + 名称)
	if 错误 != nil {
		return 凭据包.无效错误
	}
	值 := 通用凭据{类型: 1, 目标名称: 目标, 凭据二进制大对象大小: uint32(len(键)), 凭据二进制大对象: &键[0], 持久化: 2}
	成功, _, _ := 写入.调用(uintptr(不安全操作包.指针(&值)), 0)
	运行时包.保留存活(键)
	if 成功 == 0 {
		return 凭据包.不可用错误
	}
	return 执行上下文.获取错误()
}

func (存储) 加载(执行上下文 上下文包.上下文, 名称 string) (键 []byte, 返回错误 error) {
	if 错误 := 执行上下文.获取错误(); 错误 != nil {
		return nil, 错误
	}
	if !有效名称(名称) {
		return nil, 凭据包.无效错误
	}
	库, 错误 := 凭据动态链接库()
	if 错误 != nil {
		return nil, 错误
	}
	defer func() { 返回错误 = 错误判断包.合并(返回错误, 库.释放()) }()
	读取, 错误 := 库.查找进程("CredReadW")
	if 错误 != nil {
		return nil, 凭据包.不可用错误
	}
	释放, 错误 := 库.查找进程("CredFree")
	if 错误 != nil {
		return nil, 凭据包.不可用错误
	}
	目标, 错误 := Windows包.UTF16指针来源字符串(服务 + ":" + 名称)
	if 错误 != nil {
		return nil, 凭据包.无效错误
	}
	var 值 *通用凭据
	成功, _, 调用错误 := 读取.调用(uintptr(不安全操作包.指针(目标)), 1, 0, uintptr(不安全操作包.指针(&值)))
	if 成功 == 0 {
		if 错误判断包.是否属于(调用错误, Windows包.错误不已找到) {
			return nil, 凭据包.未找到错误
		}
		return nil, 凭据包.不可用错误
	}
	defer 释放.调用(uintptr(不安全操作包.指针(值)))
	if 值 == nil || 值.凭据二进制大对象大小 != 32 || 值.凭据二进制大对象 == nil {
		return nil, 凭据包.无效错误
	}
	if 错误 := 执行上下文.获取错误(); 错误 != nil {
		return nil, 错误
	}
	return append([]byte(nil), 不安全操作包.切片(值.凭据二进制大对象, 32)...), nil
}
```
