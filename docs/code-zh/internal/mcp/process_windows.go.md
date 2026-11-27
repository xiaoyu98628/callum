```go
package MCP

import (
	错误判断包 "errors"
	进程执行包 "os/exec"
	系统调用包 "syscall"
	不安全操作包 "unsafe"

	Windows包 "golang.org/x/sys/windows"
)

func 开始进程(命令 *进程执行包.命令, _ <-chan struct{}) (func() error, error) {
	作业, 错误 := Windows包.创建作业对象(nil, nil)
	if 错误 != nil {
		return nil, 错误
	}
	信息 := Windows包.作业对象扩展上限信息{}
	信息.基础上限信息.上限参数集 = Windows包.作业对象上限终止开启作业关闭
	_, 错误 = Windows包.设置信息作业对象(作业, Windows包.作业对象扩展上限信息, uintptr(不安全操作包.指针(&信息)), uint32(不安全操作包.大小(信息)))
	if 错误 != nil {
		return nil, 错误判断包.合并(错误, Windows包.关闭句柄(作业))
	}
	命令.系统进程属性 = &系统调用包.系统进程属性{创建参数集: Windows包.创建已挂起, 隐藏窗口: true}
	if 错误 := 命令.启动(); 错误 != nil {
		return nil, 错误判断包.合并(错误, Windows包.关闭句柄(作业))
	}
	进程标识 := uint32(命令.进程.进程标识)
	进程, 错误 := Windows包.打开进程(Windows包.进程设置配额|Windows包.进程终止, false, 进程标识)
	if 错误 == nil {
		错误 = 错误判断包.合并(Windows包.分配进程目标作业对象(作业, 进程), Windows包.关闭句柄(进程))
	}
	if 错误 == nil {
		错误 = 继续初始线程(进程标识)
	}
	if 错误 != nil {
		return nil, 错误判断包.合并(错误, 命令.进程.终止(), 命令.等待(), Windows包.关闭句柄(作业))
	}
	return func() error { return Windows包.关闭句柄(作业) }, nil
}

// 进程创建时挂起，先加入 作业，避免后代在回收边界建立前启动。
func 继续初始线程(进程标识 uint32) (返回错误 error) {
	快照, 错误 := Windows包.创建工具帮助32快照(Windows包.快照句柄32字符集线程快照, 0)
	if 错误 != nil {
		return 错误
	}
	defer func() { 返回错误 = 错误判断包.合并(返回错误, Windows包.关闭句柄(快照)) }()
	条目 := Windows包.线程条目32{}
	条目.大小 = uint32(不安全操作包.大小(条目))
	for 错误 = Windows包.线程32首个(快照, &条目); 错误 == nil; 错误 = Windows包.线程32下一个(快照, &条目) {
		if 条目.所属对象进程标识 != 进程标识 {
			continue
		}
		线程, 错误 := Windows包.打开线程(Windows包.线程挂起继续, false, 条目.线程标识)
		if 错误 != nil {
			return 错误
		}
		_, 错误 = Windows包.继续线程(线程)
		return 错误判断包.合并(错误, Windows包.关闭句柄(线程))
	}
	return 错误
}
```
