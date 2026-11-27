```go
package 运行

import (
	错误判断包 "errors"
	格式化包 "fmt"
	操作系统包 "os"
	运行时包 "runtime"
	字符串包 "strings"
	时间包 "time"
	UTF16包 "unicode/utf16"
	不安全操作包 "unsafe"

	Windows包 "golang.org/x/sys/windows"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

type 进程 struct {
	句柄, 线程, 作业 Windows包.句柄
	进程标识                 int
	已结束            bool
	退出                进程退出
}

func (当前进程 *进程) 获取进程标识() int { return 当前进程.进程标识 }

func 打开目标路径(路径 string) (*操作系统包.文件, error) {
	名称, 错误 := Windows包.UTF16指针来源字符串(路径)
	if 错误 != nil {
		return nil, 错误
	}
	句柄, 错误 := Windows包.创建文件(名称, Windows包.文件读取属性集, Windows包.文件共享读取|Windows包.文件共享写入|Windows包.文件共享删除, nil, Windows包.打开已有, Windows包.文件命令行参数备份语义|Windows包.文件命令行参数打开重解析点, 0)
	if 错误 != nil {
		return nil, 错误
	}
	return 操作系统包.新建文件(uintptr(句柄), 路径), nil
}

func 开始进程(固定 *固定命令, 标准输入, 标准输出, 标准错误输出 *操作系统包.文件) (已开始 *进程, 返回错误 error) {
	var 作业名称 *uint16
	var 错误 error
	if 固定.边界.沙箱内 {
		作业名称, 错误 = Windows包.UTF16指针来源字符串(沙箱作业名称(固定.沙箱状态.恢复日志.名称))
		if 错误 != nil {
			return nil, 错误
		}
	}
	作业, 错误 := Windows包.创建作业对象(nil, 作业名称)
	if 错误 != nil {
		return nil, 错误
	}
	defer func() {
		if 已开始 == nil {
			返回错误 = 错误判断包.合并(返回错误, Windows包.关闭句柄(作业))
		}
	}()
	上限集 := Windows包.作业对象扩展上限信息{}
	上限集.基础上限信息.上限参数集 = Windows包.作业对象上限终止开启作业关闭
	// 横坐标/系统 的 作业 信息参数是 uintptr，显式固定内存，不能让栈移动或 GC 改变地址。
	var 置顶 运行时包.固定器
	置顶.固定(&上限集)
	defer 置顶.解除固定()
	if _, 错误 := Windows包.设置信息作业对象(作业, Windows包.作业对象扩展上限信息, uintptr(不安全操作包.指针(&上限集)), uint32(不安全操作包.大小(上限集))); 错误 != nil {
		return nil, 错误
	}
	属性数量 := uint32(2)
	if 固定.边界.沙箱内 {
		属性数量++
	}
	属性集, 错误 := Windows包.新建进程线程属性列表(属性数量)
	if 错误 != nil {
		return nil, 错误
	}
	defer 属性集.删除()
	var 句柄集 []Windows包.句柄
	defer func() {
		for _, 句柄 := range 句柄集 {
			返回错误 = 错误判断包.合并(返回错误, Windows包.关闭句柄(句柄))
		}
	}()
	for _, 文件 := range []*操作系统包.文件{标准输入, 标准输出, 标准错误输出} {
		var 句柄 Windows包.句柄
		if 错误 := Windows包.重复句柄(Windows包.当前进程(), Windows包.句柄(文件.文件描述符()), Windows包.当前进程(), &句柄, 0, true, Windows包.重复相同访问); 错误 != nil {
			return nil, 错误
		}
		句柄集 = append(句柄集, 句柄)
	}
	if 错误 := 属性集.更新(Windows包.进程线程属性句柄列表, 不安全操作包.指针(&句柄集[0]), uintptr(len(句柄集))*不安全操作包.大小(句柄集[0])); 错误 != nil {
		return nil, 错误
	}
	// Windows 10+ 的创建时 作业 关联消除“启动后再加入”的运行窗口。
	// 属性值来自 Windows SDK winbase.处理器 / 进程线程属性作业列表（13）。
	const 进程线程属性作业列表 = 0x0002000d
	作业集 := []Windows包.句柄{作业}
	if 错误 := 属性集.更新(进程线程属性作业列表, 不安全操作包.指针(&作业集[0]), 不安全操作包.大小(作业集[0])); 错误 != nil {
		return nil, 错误
	}
	能力集 := 安全能力集{SID: 固定.沙箱状态.SID}
	if 固定.边界.沙箱内 {
		if !固定.沙箱状态.活动 || 能力集.SID == nil || 固定.沙箱状态.词元 == 0 {
			return nil, 工具包.已拒绝错误
		}
		置顶.固定(能力集.SID)
		置顶.固定(&能力集)
		const 进程线程属性安全能力集 = 0x00020009
		if 错误 := 属性集.更新(进程线程属性安全能力集, 不安全操作包.指针(&能力集), 不安全操作包.大小(能力集)); 错误 != nil {
			return nil, 错误
		}
	}
	应用, 错误 := Windows包.UTF16指针来源字符串(固定.可执行文件.实际)
	if 错误 != nil {
		return nil, 错误
	}
	行, 错误 := Windows包.UTF16指针来源字符串(Windows包.组合命令行(固定.命令))
	if 错误 != nil {
		return nil, 错误
	}
	工作目录, 错误 := Windows包.UTF16指针来源字符串(固定.目录.实际)
	if 错误 != nil {
		return nil, 错误
	}
	环境 := UTF16包.编码([]rune(字符串包.合并(固定.环境, "\x00") + "\x00\x00"))
	启动 := Windows包.启动信息本次执行{进程线程属性列表: 属性集.列表()}
	启动.回调 = uint32(不安全操作包.大小(启动))
	启动.参数集 = Windows包.启动标志使用标准句柄
	启动.标准输入, 启动.标准输出, 启动.标准错误 = 句柄集[0], 句柄集[1], 句柄集[2]
	var 原生 Windows包.进程信息
	参数集 := uint32(Windows包.创建Unicode环境 | Windows包.扩展启动信息存在 | Windows包.创建无窗口)
	if 固定.边界.沙箱内 {
		错误 = Windows包.创建进程作为用户(固定.沙箱状态.词元, 应用, 行, nil, nil, true, 参数集|Windows包.创建已挂起, &环境[0], 工作目录, &启动.启动信息, &原生)
	} else {
		错误 = Windows包.创建进程(应用, 行, nil, nil, true, 参数集, &环境[0], 工作目录, &启动.启动信息, &原生)
	}
	运行时包.保留存活(环境)
	运行时包.保留存活(句柄集)
	运行时包.保留存活(作业集)
	if 错误 != nil {
		return nil, 错误
	}
	if 固定.边界.沙箱内 {
		核验错误 := 核验沙箱进程(原生.进程, 固定.沙箱状态.SID)
		if 核验错误 == nil {
			_, 核验错误 = Windows包.继续线程(原生.线程)
		}
		if 核验错误 != nil {
			return nil, 错误判断包.合并(核验错误, Windows包.终止作业对象(作业, 1), Windows包.关闭句柄(原生.线程), Windows包.关闭句柄(原生.进程))
		}
	}
	return &进程{句柄: 原生.进程, 线程: 原生.线程, 作业: 作业, 进程标识: int(原生.进程标识)}, nil
}

func (当前进程 *进程) 轮询() (进程退出, bool, error) {
	if 当前进程.已结束 {
		return 当前进程.退出, true, nil
	}
	状态, 错误 := Windows包.等待用于单个对象(当前进程.句柄, 0)
	if 错误 != nil {
		return 进程退出{}, false, 错误
	}
	if 状态 == uint32(Windows包.等待超时) {
		return 进程退出{}, false, nil
	}
	if 状态 != Windows包.等待对象0 {
		return 进程退出{}, false, 工具包.不可用错误
	}
	var 代码 uint32
	if 错误 := Windows包.获取退出代码进程(当前进程.句柄, &代码); 错误 != nil {
		return 进程退出{}, false, 错误
	}
	当前进程.已结束, 当前进程.退出.代码 = true, int(代码)
	return 当前进程.退出, true, nil
}

func (当前进程 *进程) 停止() error { return Windows包.终止作业对象(当前进程.作业, 1) }

func (当前进程 *进程) close() error {
	错误 := 当前进程.停止()
	// 作业 的终止请求不等于全部成员已经退出，读取原生计数后再报告清理结果。
	截止时间 := 时间包.当前时间().添加(清理超时)
	for {
		空, 查询错误 := 作业空(当前进程.作业)
		if 查询错误 != nil {
			错误 = 错误判断包.合并(错误, 查询错误)
			break
		}
		if 空 {
			break
		}
		if 时间包.当前时间().之后(截止时间) {
			错误 = 错误判断包.合并(错误, 工具包.不可用错误)
			break
		}
		时间包.休眠(10 * 时间包.毫秒)
	}
	return 错误判断包.合并(错误, Windows包.关闭句柄(当前进程.线程), Windows包.关闭句柄(当前进程.句柄), Windows包.关闭句柄(当前进程.作业))
}

func 作业空(作业 Windows包.句柄) (bool, error) {
	计量 := struct {
		总计用户, 总计内核, 周期用户, 周期内核                 int64
		页故障集, 总计进程集, 活动进程集, 已终止进程集 uint32
	}{}
	var 置顶 运行时包.固定器
	置顶.固定(&计量)
	defer 置顶.解除固定()
	错误 := Windows包.查询信息作业对象(作业, Windows包.作业对象基础计量信息, uintptr(不安全操作包.指针(&计量)), uint32(不安全操作包.大小(计量)), nil)
	return 计量.活动进程集 == 0, 错误
}

func 文件身份(文件 *操作系统包.文件, _ 操作系统包.文件信息) (string, error) {
	var 信息 Windows包.依据句柄文件信息
	if 错误 := Windows包.获取文件信息依据句柄(Windows包.句柄(文件.文件描述符()), &信息); 错误 != nil {
		return "", 错误判断包.合并(工具包.不可用错误, 错误)
	}
	return 格式化包.格式化字符串("%d:%d:%d", 信息.卷串行数量, 信息.文件索引高, 信息.文件索引低), nil
}
```
