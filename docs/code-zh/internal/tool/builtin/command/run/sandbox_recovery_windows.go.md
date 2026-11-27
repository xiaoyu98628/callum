```go
package 运行

import (
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"
	格式化包 "fmt"
	输入输出包 "io"
	文件系统包 "io/fs"
	操作系统包 "os"
	文件路径包 "path/filepath"
	运行时包 "runtime"
	字符串包 "strings"
	时间包 "time"
	不安全操作包 "unsafe"

	Windows包 "golang.org/x/sys/windows"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

func 确保沙箱恢复日志目录(路径 string) error {
	已创建 := false
	if 错误 := 操作系统包.创建目录(路径, 0700); 错误 != nil {
		if !错误判断包.是否属于(错误, 操作系统包.存在错误) {
			return 错误
		}
	} else {
		已创建 = true
	}
	信息, 错误 := 操作系统包.读取链接自身状态(路径)
	if 错误 != nil {
		return 错误
	}
	if !信息.是否目录() || 信息.模式()&操作系统包.模式符号链接 != 0 {
		return 工具包.已拒绝错误
	}
	用户, 错误 := Windows包.获取当前进程词元().获取词元用户()
	if 错误 != nil {
		return 错误
	}
	if 已创建 {
		return 安全新建沙箱目录(路径)
	}
	安全描述符, 错误 := Windows包.获取命名安全信息(路径, Windows包.步骤错误文件对象, Windows包.所属对象安全信息|Windows包.DACL安全信息)
	if 错误 != nil {
		return 错误
	}
	所属对象, _, 错误 := 安全描述符.所属对象()
	if 错误 != nil {
		return 错误
	}
	if !所属对象.等于(用户.用户.SID) {
		return 工具包.已拒绝错误
	}
	ACL, _, 错误 := 安全描述符.DACL()
	if 错误 != nil || ACL == nil {
		return 错误判断包.合并(工具包.已拒绝错误, 错误)
	}
	系统, 错误 := Windows包.创建正确已知SID(Windows包.Windows本地系统SID)
	if 错误 != nil {
		return 错误
	}
	for 索引 := uint32(0); 索引 < uint32(ACL.访问控制项数量); 索引++ {
		var 访问控制项 *Windows包.访问允许访问控制项
		if 错误 := Windows包.获取访问控制项(ACL, 索引, &访问控制项); 错误 != nil {
			return 错误
		}
		if 访问控制项.头部.访问控制项类型 == Windows包.访问已拒绝访问控制项类型 {
			continue
		}
		// 私有恢复目录不接受无法验证受托人的回调或对象专用授权条目。
		if 访问控制项.头部.访问控制项类型 != Windows包.访问允许访问控制项类型 {
			return 工具包.已拒绝错误
		}
		SID := (*Windows包.SID)(不安全操作包.指针(&访问控制项.SID开始))
		if !SID.等于(用户.用户.SID) && !SID.等于(系统) {
			return 工具包.已拒绝错误
		}
	}
	return nil
}

func 安全新建沙箱目录(路径 string) error {
	用户, 错误 := Windows包.获取当前进程词元().获取词元用户()
	if 错误 != nil {
		return 错误
	}
	安全描述符, 错误 := Windows包.安全描述符来源字符串("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;" + 用户.用户.SID.字符串() + ")")
	if 错误 != nil {
		return 错误
	}
	ACL, _, 错误 := 安全描述符.DACL()
	if 错误 != nil {
		return 错误
	}
	return Windows包.设置命名安全信息(路径, Windows包.步骤错误文件对象, Windows包.所属对象安全信息|Windows包.DACL安全信息|Windows包.受保护DACL安全信息, 用户.用户.SID, nil, ACL, nil)
}

func 写入沙箱恢复日志(路径 string, 恢复日志 沙箱恢复日志) (返回错误 error) {
	数据, 错误 := JSON包.序列化(恢复日志)
	if 错误 != nil {
		return 错误
	}
	if len(数据) > 最大沙箱恢复日志字节 {
		return 工具包.超出上限错误
	}
	文件, 错误 := 操作系统包.打开文件(路径, 操作系统包.仅写打开标志|操作系统包.创建文件标志|操作系统包.独占创建标志, 0600)
	if 错误 != nil {
		return 错误
	}
	defer func() {
		返回错误 = 错误判断包.合并(返回错误, 文件.关闭())
		if 返回错误 != nil {
			返回错误 = 错误判断包.合并(返回错误, 操作系统包.移除(路径))
		}
	}()
	if _, 错误 := 文件.写入(数据); 错误 != nil {
		return 错误
	}
	return 文件.同步()
}

func 带有沙箱ACL加锁(操作 func() error) (返回错误 error) {
	// Windows 互斥锁 归操作系统线程所有，持有期间不能让 goroutine 切换线程。
	运行时包.加锁操作系统线程()
	defer 运行时包.解锁操作系统线程()
	用户, 错误 := Windows包.获取当前进程词元().获取词元用户()
	if 错误 != nil {
		return 错误
	}
	名称, 错误 := Windows包.UTF16指针来源字符串(`Global\CallumSandboxACL.` + 用户.用户.SID.字符串())
	if 错误 != nil {
		return 错误
	}
	互斥锁, 错误 := Windows包.创建互斥锁(nil, false, 名称)
	if 错误 != nil && !错误判断包.是否属于(错误, Windows包.错误已经存在) {
		return 错误
	}
	defer func() { 返回错误 = 错误判断包.合并(返回错误, Windows包.关闭句柄(互斥锁)) }()
	状态, 错误 := Windows包.等待用于单个对象(互斥锁, 5000)
	if 错误 != nil {
		return 错误
	}
	if 状态 != Windows包.等待对象0 && 状态 != Windows包.等待已遗弃 {
		return 工具包.不可用错误
	}
	defer func() { 返回错误 = 错误判断包.合并(返回错误, Windows包.释放互斥锁(互斥锁)) }()
	return 操作()
}

func 沙箱作业名称(配置档 string) string { return `Global\CallumSandbox.` + 配置档 }

// 停止沙箱作业 在回收权限前确认旧进程树已经结束，避免清理期间继续执行。
func 停止沙箱作业(配置档 string) (返回错误 error) {
	名称, 错误 := Windows包.UTF16指针来源字符串(沙箱作业名称(配置档))
	if 错误 != nil {
		return 错误
	}
	进程 := Windows包.新建延迟系统动态链接库("kernel32.dll").新建进程("OpenJobObjectW")
	结果, _, 调用错误 := 进程.调用(0x4|0x8, 0, uintptr(不安全操作包.指针(名称)))
	if 结果 == 0 {
		if 错误判断包.是否属于(调用错误, Windows包.错误文件不已找到) {
			return nil
		}
		return 调用错误
	}
	作业 := Windows包.句柄(结果)
	defer func() { 返回错误 = 错误判断包.合并(返回错误, Windows包.关闭句柄(作业)) }()
	if 错误 := Windows包.终止作业对象(作业, 1); 错误 != nil {
		return 错误
	}
	for 截止时间 := 时间包.当前时间().添加(清理超时); ; {
		空, 错误 := 作业空(作业)
		if 错误 != nil {
			return 错误
		}
		if 空 {
			return nil
		}
		if 时间包.当前时间().之后(截止时间) {
			return 工具包.不可用错误
		}
		时间包.休眠(10 * 时间包.毫秒)
	}
}

func (当前沙箱状态 *沙箱状态) 清理() error {
	var 错误 error
	if 当前沙箱状态.词元 != 0 {
		错误 = 当前沙箱状态.词元.关闭()
		当前沙箱状态.词元 = 0
	}
	if 当前沙箱状态.活动 {
		if 停止错误 := 停止沙箱作业(当前沙箱状态.恢复日志.名称); 停止错误 != nil {
			return 错误判断包.合并(错误, 停止错误)
		}
		if 当前沙箱状态.配置档自有 {
			错误 = 错误判断包.合并(错误, 移除沙箱配置档(当前沙箱状态.恢复日志.名称))
			if 错误 != nil {
				return 错误
			}
			当前沙箱状态.配置档自有 = false
		}
		if 当前沙箱状态.ACL已触及 {
			错误 = 错误判断包.合并(错误, 带有沙箱ACL加锁(func() error {
				var 清理错误 error
				for _, 条目 := range 当前沙箱状态.恢复日志.条目集 {
					清理错误 = 错误判断包.合并(清理错误, 更新沙箱ACL(条目, 当前沙箱状态.SID, true))
				}
				// 新建及项目内改名的对象可能继承 SID；进程树结束后统一回收。
				根目录集 := append([]沙箱根(nil), 当前沙箱状态.恢复日志.根目录集...)
				根目录集 = append(根目录集, 沙箱根{路径: 当前沙箱状态.恢复日志.临时}, 沙箱根{路径: 当前沙箱状态.恢复日志.受保护目录})
				for _, 根 := range 根目录集 {
					if 根.环境 {
						continue
					}
					已访问 := 0
					遍历错误 := 文件路径包.遍历目录(根.路径, func(路径 string, 条目 文件系统包.目录条目, 遍历错误 error) error {
						if 错误判断包.是否属于(遍历错误, 操作系统包.不存在错误) {
							return nil
						}
						if 遍历错误 != nil {
							return 遍历错误
						}
						已访问++
						if 已访问 > 最大沙箱条目集 {
							return 工具包.超出上限错误
						}
						return 更新沙箱ACL(沙箱ACL条目{路径: 路径, 目录: 条目.是否目录()}, 当前沙箱状态.SID, true)
					})
					清理错误 = 错误判断包.合并(清理错误, 遍历错误)
				}
				return 清理错误
			}))
		}
		if 错误 != nil {
			return 格式化包.格式化创建错误("Windows 沙箱权限回收未完成，恢复记录保留于 %s: %w", 当前沙箱状态.恢复日志路径, 错误)
		}
	}
	if 当前沙箱状态.恢复日志.临时 != "" {
		for _, 条目 := range 当前沙箱状态.恢复日志.条目集 {
			if !字符串包.相等折叠(条目.路径, 当前沙箱状态.恢复日志.临时) {
				continue
			}
			身份, 身份错误 := 沙箱路径身份(条目.路径)
			if 身份错误 != nil && !错误判断包.是否属于(身份错误, 操作系统包.不存在错误) {
				return 身份错误
			}
			if 身份错误 == nil && 身份 != 条目.身份 {
				return 工具包.冲突错误
			}
		}
		错误 = 错误判断包.合并(错误, 操作系统包.移除全部(当前沙箱状态.恢复日志.临时))
	}
	if 错误 != nil {
		return 错误
	}
	if 当前沙箱状态.活动 {
		if 错误 := 操作系统包.移除(当前沙箱状态.恢复日志路径); 错误 != nil {
			return 错误
		}
		当前沙箱状态.活动 = false
	}
	return nil
}

// 恢复沙箱 在实例锁保护下回收中断执行的 Windows 沙箱，失败时阻止应用继续初始化。
func 恢复沙箱(执行上下文 上下文包.上下文, 数据目录 string) error {
	目录 := 文件路径包.合并(数据目录, "command-sandboxes")
	if _, 错误 := 操作系统包.读取链接自身状态(目录); 错误判断包.是否属于(错误, 操作系统包.不存在错误) {
		return nil
	} else if 错误 != nil {
		return 错误
	}
	数据目录, 错误 := 文件路径包.求值符号链接集(数据目录)
	if 错误 != nil {
		return 错误
	}
	目录 = 文件路径包.合并(数据目录, "command-sandboxes")
	if 错误 := 确保沙箱恢复日志目录(目录); 错误 != nil {
		return 错误
	}
	条目集, 错误 := 操作系统包.读取目录(目录)
	if 错误 != nil {
		return 错误
	}
	for _, 条目 := range 条目集 {
		if 错误 := 执行上下文.获取错误(); 错误 != nil {
			return 错误
		}
		if 条目.是否目录() || 条目.类型()&操作系统包.模式符号链接 != 0 || !字符串包.存在后缀(条目.名称(), ".json") {
			return 工具包.已拒绝错误
		}
		路径 := 文件路径包.合并(目录, 条目.名称())
		文件, 错误 := 操作系统包.打开(路径)
		if 错误 != nil {
			return 错误
		}
		数据, 读取错误 := 输入输出包.读取全部(输入输出包.上限读取器(文件, 最大沙箱恢复日志字节+1))
		if 错误 := 错误判断包.合并(读取错误, 文件.关闭()); 错误 != nil {
			return 错误
		}
		if len(数据) > 最大沙箱恢复日志字节 {
			return 工具包.超出上限错误
		}
		var 恢复日志 沙箱恢复日志
		解码器 := JSON包.新建解码器(字符串包.新建读取器(string(数据)))
		解码器.禁止未知字段集()
		if 错误 := 解码器.解码(&恢复日志); 错误 != nil {
			return 错误
		}
		if 解码器.解码(new(any)) != 输入输出包.文件结束 {
			return 工具包.无效错误
		}
		if 恢复日志.版本 != 沙箱恢复日志版本 || !字符串包.存在前缀(恢复日志.名称, "callum.command.") || 条目.名称() != 恢复日志.名称+".json" || len(恢复日志.名称) != len("callum.command.")+26 {
			return 工具包.无效错误
		}
		if !字符串包.相等折叠(恢复日志.受保护目录, 数据目录) {
			return 工具包.已拒绝错误
		}
		SID, 错误 := 沙箱SID(恢复日志.名称)
		if 错误 != nil {
			return 错误
		}
		if SID.字符串() != 恢复日志.SID {
			return 工具包.已拒绝错误
		}
		if !文件路径包.是否绝对路径(恢复日志.临时) || !字符串包.存在前缀(文件路径包.基准(恢复日志.临时), "callum-command-") || len(文件路径包.基准(恢复日志.临时)) != len("callum-command-")+26 {
			return 工具包.已拒绝错误
		}
		if len(恢复日志.条目集) > 最大沙箱条目集 || len(恢复日志.根目录集) > 最大沙箱根目录集 {
			return 工具包.超出上限错误
		}
		for _, 根 := range 恢复日志.根目录集 {
			if !文件路径包.是否绝对路径(根.路径) {
				return 工具包.无效错误
			}
		}
		for _, ACL条目 := range 恢复日志.条目集 {
			if !文件路径包.是否绝对路径(ACL条目.路径) {
				return 工具包.无效错误
			}
			允许 := 范围内沙箱路径(ACL条目.路径, 数据目录) || 范围内沙箱路径(ACL条目.路径, 恢复日志.临时)
			for _, 根 := range 恢复日志.根目录集 {
				允许 = 允许 || 范围内沙箱路径(ACL条目.路径, 根.路径)
			}
			if !允许 {
				return 工具包.已拒绝错误
			}
		}
		状态 := 沙箱状态{恢复日志: 恢复日志, 恢复日志路径: 路径, SID: SID, 活动: true, 配置档自有: true, ACL已触及: true}
		if 错误 := 状态.清理(); 错误 != nil {
			return 格式化包.格式化创建错误("回收 Windows 沙箱失败: %w", 错误)
		}
	}
	return nil
}

func 范围内沙箱路径(路径, 根 string) bool {
	相对, 错误 := 文件路径包.相对路径(字符串包.目标小写(根), 字符串包.目标小写(路径))
	return 错误 == nil && 文件路径包.是否本地路径(相对)
}
```
