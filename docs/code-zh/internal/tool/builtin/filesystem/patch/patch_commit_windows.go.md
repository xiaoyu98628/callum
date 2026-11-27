```go
package 补丁

import (
	错误判断包 "errors"
	操作系统包 "os"
	系统调用包 "syscall"

	Windows包 "golang.org/x/sys/windows"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

func 补丁文件就绪(文件 *操作系统包.文件) error {
	信息, 错误 := 文件.状态信息()
	if 错误 != nil {
		return 工具包.不可用错误
	}
	var 原生 Windows包.依据句柄文件信息
	if Windows包.获取文件信息依据句柄(Windows包.句柄(文件.文件描述符()), &原生) != nil {
		return 工具包.不可用错误
	}
	if !信息.模式().是否普通文件() || 原生.数量所属链接集 != 1 || 原生.文件属性集&Windows包.文件属性重解析点 != 0 {
		return 工具包.已拒绝错误
	}
	return nil
}

// 新文件和恢复副本使用当前进程用户的受保护 DACL，不继承父目录的宽松权限。
// 已有文件通过原句柄更新，保留其原有 DACL、所有者及其他权限。
func 保护补丁文件(文件 *操作系统包.文件) (返回错误 error) {
	用户, 错误 := Windows包.获取当前进程词元().获取词元用户()
	if 错误 != nil {
		return 错误
	}
	安全描述符, 错误 := Windows包.安全描述符来源字符串("D:P(A;;FA;;;" + 用户.用户.SID.字符串() + ")")
	if 错误 != nil {
		return 错误
	}
	DACL, _, 错误 := 安全描述符.DACL()
	if 错误 != nil {
		return 错误
	}
	// 操作系统.根.打开文件 的数据句柄未请求 写入自主访问控制。按原句柄重新打开相同
	// 对象以修改新建对象的权限，不用路径重开，避免路径替换导致误改权限。
	库, 错误 := Windows包.加载库本次执行("kernel32.dll", 0, Windows包.加载库搜索系统32)
	if 错误 != nil {
		return 错误
	}
	defer func() { 返回错误 = 错误判断包.合并(返回错误, Windows包.释放库(库)) }()
	地址, 错误 := Windows包.获取进程地址(库, "ReOpenFile")
	if 错误 != nil {
		return 错误
	}
	值, _, 错误 := 系统调用包.系统调用数量(地址, 文件.文件描述符(), Windows包.写入自主访问控制|Windows包.读取控制,
		Windows包.文件共享读取|Windows包.文件共享写入|Windows包.文件共享删除, Windows包.文件命令行参数备份语义)
	句柄 := Windows包.句柄(值)
	if 句柄 == Windows包.无效句柄 {
		return 错误
	}
	defer func() { 返回错误 = 错误判断包.合并(返回错误, Windows包.关闭句柄(句柄)) }()
	return Windows包.设置安全信息(句柄, Windows包.步骤错误文件对象, Windows包.DACL安全信息|Windows包.受保护DACL安全信息, nil, nil, DACL, nil)
}
func 保护补丁目录(文件 *操作系统包.文件) error { return 保护补丁文件(文件) }
```
