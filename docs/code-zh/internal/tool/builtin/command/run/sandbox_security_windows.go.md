```go
package 运行

import (
	二进制包 "encoding/binary"
	错误判断包 "errors"
	格式化包 "fmt"
	操作系统包 "os"
	运行时包 "runtime"
	不安全操作包 "unsafe"

	Windows包 "golang.org/x/sys/windows"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

const 沙箱文件全部访问 = 0x001f01ff
const 沙箱删除子级 = 0x40

var (
	沙箱用户环境    = Windows包.新建延迟系统动态链接库("userenv.dll")
	创建应用容器配置档 = 沙箱用户环境.新建进程("CreateAppContainerProfile")
	删除应用容器配置档 = 沙箱用户环境.新建进程("DeleteAppContainerProfile")
	派生应用容器SID = 沙箱用户环境.新建进程("DeriveAppContainerSidFromAppContainerName")
	创建受限词元    = Windows包.新建延迟系统动态链接库("advapi32.dll").新建进程("CreateRestrictedToken")
)

// 安全能力集 使用 Windows SDK 布局，适用于 横坐标64 和 ARM64。
type 安全能力集 struct {
	SID *Windows包.SID
	能力集 *Windows包.SID并且属性集
	数量  uint32
	预留  uint32
}

func 沙箱结果码(结果 uintptr) error {
	if int32(结果) >= 0 {
		return nil
	}
	if uint32(结果)>>16&0x1fff == 7 {
		return Windows包.系统错误码(uint32(结果) & 0xffff)
	}
	return 格式化包.格式化创建错误("Windows 沙箱 API 返回 HRESULT 0x%08x", uint32(结果))
}

func 沙箱SID(名称 string) (*Windows包.SID, error) {
	宽字符, 错误 := Windows包.UTF16指针来源字符串(名称)
	if 错误 != nil {
		return nil, 错误
	}
	var SID *Windows包.SID
	结果, _, _ := 派生应用容器SID.调用(uintptr(不安全操作包.指针(宽字符)), uintptr(不安全操作包.指针(&SID)))
	if 错误 := 沙箱结果码(结果); 错误 != nil {
		return nil, 错误
	}
	copy, 复制错误 := SID.复制()
	return copy, 错误判断包.合并(复制错误, Windows包.释放SID(SID))
}

func 新建沙箱配置档(名称 string, 预期 *Windows包.SID) (bool, error) {
	宽字符, 错误 := Windows包.UTF16指针来源字符串(名称)
	if 错误 != nil {
		return false, 错误
	}
	var SID *Windows包.SID
	结果, _, _ := 创建应用容器配置档.调用(uintptr(不安全操作包.指针(宽字符)), uintptr(不安全操作包.指针(宽字符)), uintptr(不安全操作包.指针(宽字符)), 0, 0, uintptr(不安全操作包.指针(&SID)))
	if 错误 := 沙箱结果码(结果); 错误 != nil {
		return false, 错误
	}
	var 身份错误 error
	if !SID.等于(预期) {
		身份错误 = 工具包.已拒绝错误
	}
	return true, 错误判断包.合并(身份错误, Windows包.释放SID(SID))
}

func 移除沙箱配置档(名称 string) error {
	宽字符, 错误 := Windows包.UTF16指针来源字符串(名称)
	if 错误 != nil {
		return 错误
	}
	结果, _, _ := 删除应用容器配置档.调用(uintptr(不安全操作包.指针(宽字符)))
	错误 = 沙箱结果码(结果)
	if 错误判断包.是否属于(错误, Windows包.错误文件不已找到) || 错误判断包.是否属于(错误, Windows包.错误路径不已找到) || 错误判断包.是否属于(错误, Windows包.错误不已找到) {
		return nil
	}
	return 错误
}

func 沙箱受限词元(SID *Windows包.SID) (Windows包.词元, error) {
	var 原始, 受限 Windows包.词元
	if 错误 := Windows包.打开进程词元(Windows包.当前进程(), Windows包.词元重复|Windows包.词元查询|Windows包.词元分配主要, &原始); 错误 != nil {
		return 0, 错误
	}
	条目 := Windows包.SID并且属性集{SID: SID}
	var 置顶 运行时包.固定器
	置顶.固定(SID)
	置顶.固定(&条目)
	defer 置顶.解除固定()
	// 写入受限 让任意写入还必须通过本次唯一 SID 的访问检查。
	// 不把 Everyone 或登录用户加入限制 SID，避免继承其他执行的可写范围。
	结果, _, 调用错误 := 创建受限词元.调用(uintptr(原始), 0x1|0x8, 0, 0, 0, 0, 1, uintptr(不安全操作包.指针(&条目)), uintptr(不安全操作包.指针(&受限)))
	运行时包.保留存活(SID)
	错误 := 原始.关闭()
	if 结果 == 0 {
		return 0, 错误判断包.合并(调用错误, 错误)
	}
	if 错误 != nil {
		return 0, 错误判断包.合并(错误, 受限.关闭())
	}
	return 受限, nil
}

func 核验沙箱进程(进程 Windows包.句柄, 预期 *Windows包.SID) (返回错误 error) {
	var 词元 Windows包.词元
	if 错误 := Windows包.打开进程词元(进程, Windows包.词元查询, &词元); 错误 != nil {
		return 错误
	}
	defer func() { 返回错误 = 错误判断包.合并(返回错误, 词元.关闭()) }()
	var 应用容器, 已返回 uint32
	// TOKEN_INFORMATION_CLASS 的 词元是应用容器 与 词元应用容器SID 分别为 29、31。
	if 错误 := Windows包.获取词元信息(词元, 29, (*byte)(不安全操作包.指针(&应用容器)), 4, &已返回); 错误 != nil {
		return 错误
	}
	if 应用容器 != 1 {
		return 工具包.已拒绝错误
	}
	错误 := Windows包.获取词元信息(词元, 31, nil, 0, &已返回)
	if !错误判断包.是否属于(错误, Windows包.错误不足缓冲区) || 已返回 < uint32(不安全操作包.大小(uintptr(0))) || 已返回 > 65536 {
		return 错误判断包.合并(工具包.已拒绝错误, 错误)
	}
	缓冲区 := make([]byte, 已返回)
	if 错误 := Windows包.获取词元信息(词元, 31, &缓冲区[0], uint32(len(缓冲区)), &已返回); 错误 != nil {
		return 错误
	}
	SID := *(**Windows包.SID)(不安全操作包.指针(&缓冲区[0]))
	if SID == nil || !SID.等于(预期) {
		return 工具包.已拒绝错误
	}
	运行时包.保留存活(缓冲区)
	受限, 错误 := 词元.是受限()
	if 错误 != nil {
		return 错误
	}
	if !受限 {
		return 工具包.已拒绝错误
	}
	错误 = Windows包.获取词元信息(词元, Windows包.词元受限SID集, nil, 0, &已返回)
	if !错误判断包.是否属于(错误, Windows包.错误不足缓冲区) || 已返回 < uint32(不安全操作包.大小(Windows包.令牌组{})) || 已返回 > 65536 {
		return 错误判断包.合并(工具包.已拒绝错误, 错误)
	}
	缓冲区 = make([]byte, 已返回)
	if 错误 := Windows包.获取词元信息(词元, Windows包.词元受限SID集, &缓冲区[0], uint32(len(缓冲区)), &已返回); 错误 != nil {
		return 错误
	}
	组集 := (*Windows包.令牌组)(不安全操作包.指针(&缓冲区[0]))
	if 组集.组数量 != 1 || !组集.组集[0].SID.等于(预期) {
		return 工具包.已拒绝错误
	}
	运行时包.保留存活(缓冲区)
	return nil
}

func 打开沙箱ACL路径(路径 string, 目录, 读取仅 bool) (*操作系统包.文件, error) {
	宽字符, 错误 := Windows包.UTF16指针来源字符串(路径)
	if 错误 != nil {
		return nil, 错误
	}
	// 最大允许 阻止 设置安全信息 自动向已有子项传播权限。
	// 逐个固定对象更新，才能跳过重解析点并保留每个对象的原有 ACL。
	访问 := uint32(Windows包.读取控制 | Windows包.写入自主访问控制)
	if 目录 {
		访问 = Windows包.最大允许
	}
	if 读取仅 {
		访问 = Windows包.读取控制 | Windows包.文件读取属性集
	}
	句柄, 错误 := Windows包.创建文件(宽字符, 访问, Windows包.文件共享读取|Windows包.文件共享写入|Windows包.文件共享删除, nil, Windows包.打开已有, Windows包.文件命令行参数备份语义|Windows包.文件命令行参数打开重解析点, 0)
	if 错误 != nil {
		return nil, 错误
	}
	return 操作系统包.新建文件(uintptr(句柄), 路径), nil
}

func 沙箱路径身份(路径 string) (身份 string, 返回错误 error) {
	文件, 错误 := 打开目标路径(路径)
	if 错误 != nil {
		return "", 错误
	}
	defer func() { 返回错误 = 错误判断包.合并(返回错误, 文件.关闭()) }()
	var 信息 Windows包.依据句柄文件信息
	if 错误 := Windows包.获取文件信息依据句柄(Windows包.句柄(文件.文件描述符()), &信息); 错误 != nil {
		return "", 错误
	}
	if 信息.文件属性集&Windows包.文件属性重解析点 != 0 {
		return "", 工具包.已拒绝错误
	}
	return 文件身份(文件, nil)
}

func 更新沙箱ACL(条目 沙箱ACL条目, SID *Windows包.SID, 移除 bool) (返回错误 error) {
	文件, 错误 := 打开沙箱ACL路径(条目.路径, 条目.目录, 移除)
	if 移除 && (错误判断包.是否属于(错误, Windows包.错误文件不已找到) || 错误判断包.是否属于(错误, Windows包.错误路径不已找到)) {
		return nil
	}
	if 错误 != nil {
		return 错误
	}
	defer func() { 返回错误 = 错误判断包.合并(返回错误, 文件.关闭()) }()
	var 信息 Windows包.依据句柄文件信息
	if 错误 := Windows包.获取文件信息依据句柄(Windows包.句柄(文件.文件描述符()), &信息); 错误 != nil {
		return 错误
	}
	if !移除 && 信息.文件属性集&Windows包.文件属性重解析点 != 0 {
		return 工具包.冲突错误
	}
	目录 := 信息.文件属性集&Windows包.文件属性目录 != 0
	if !移除 && 目录 != 条目.目录 {
		return 工具包.冲突错误
	}
	身份, 错误 := 文件身份(文件, nil)
	if 错误 != nil {
		return 错误
	}
	if !移除 && 条目.身份 != "" && 身份 != 条目.身份 {
		return 工具包.冲突错误
	}
	安全描述符, 错误 := Windows包.获取安全信息(Windows包.句柄(文件.文件描述符()), Windows包.步骤错误文件对象, Windows包.DACL安全信息)
	if 错误 != nil {
		return 错误
	}
	ACL, _, 错误 := 安全描述符.DACL()
	if 错误 != nil {
		return 错误
	}
	if ACL == nil {
		if 移除 {
			return nil
		}
		return 工具包.已拒绝错误
	}
	if 移除 {
		已筛选, 变化通知, 错误 := 移除沙箱访问控制项集(ACL, SID)
		if 错误 != nil || !变化通知 {
			return 错误
		}
		访问 := uint32(Windows包.读取控制 | Windows包.写入自主访问控制)
		if 目录 {
			访问 = Windows包.最大允许
		}
		进程 := Windows包.新建延迟系统动态链接库("kernel32.dll").新建进程("ReOpenFile")
		值, _, 重新打开错误 := 进程.调用(文件.文件描述符(), uintptr(访问), Windows包.文件共享读取|Windows包.文件共享写入|Windows包.文件共享删除, Windows包.文件命令行参数备份语义|Windows包.文件命令行参数打开重解析点)
		句柄 := Windows包.句柄(值)
		if 句柄 == Windows包.无效句柄 {
			return 重新打开错误
		}
		defer func() { 返回错误 = 错误判断包.合并(返回错误, Windows包.关闭句柄(句柄)) }()
		错误 = Windows包.设置安全信息(句柄, Windows包.步骤错误文件对象, Windows包.DACL安全信息, nil, nil, (*Windows包.ACL)(不安全操作包.指针(&已筛选[0])), nil)
		运行时包.保留存活(已筛选)
		return 错误
	}
	var 置顶 运行时包.固定器
	置顶.固定(SID)
	defer 置顶.解除固定()
	受托方 := Windows包.受托方{受托方表单: Windows包.受托方是SID, 受托方类型: Windows包.受托方是未知, 受托方值: Windows包.受托方值来源SID(SID)}
	继承 := uint32(0)
	if 条目.目录 && !条目.自身仅 {
		继承 = Windows包.对象继承访问控制项 | Windows包.容器继承访问控制项
	}
	模式 := Windows包.访问模式(Windows包.授权访问)
	if 条目.拒绝 {
		模式 = Windows包.拒绝访问
	}
	条目集 := []Windows包.显式访问{{访问权限: Windows包.访问掩码(条目.掩码), 访问模式: 模式, 继承: 继承, 受托方: 受托方}}
	if 条目.目录 && !条目.拒绝 && 条目.掩码&Windows包.文件写入数据 != 0 {
		条目集 = append(条目集, Windows包.显式访问{访问权限: Windows包.删除, 访问模式: Windows包.授权访问, 继承: 继承 | Windows包.继承仅访问控制项, 受托方: 受托方})
	}
	if 条目.拒绝 && 条目.掩码 != 沙箱文件全部访问 {
		条目集 = append(条目集, Windows包.显式访问{访问权限: Windows包.文件通用读取 | Windows包.文件通用执行, 访问模式: Windows包.授权访问, 继承: 继承, 受托方: 受托方})
	}
	已更新, 错误 := Windows包.ACL来源条目集(条目集, ACL)
	if 错误 != nil {
		return 错误
	}
	return Windows包.设置安全信息(Windows包.句柄(文件.文件描述符()), Windows包.步骤错误文件对象, Windows包.DACL安全信息, nil, nil, 已更新, nil)
}

// 已有系统安装目录可通过 全部 应用 包集 提供读取，不修改这些目录的 ACL。
func 环境Windows读取(路径 string) bool {
	安全描述符, 错误 := Windows包.获取命名安全信息(路径, Windows包.步骤错误文件对象, Windows包.DACL安全信息)
	if 错误 != nil {
		return false
	}
	ACL, _, 错误 := 安全描述符.DACL()
	if 错误 != nil || ACL == nil {
		return false
	}
	SID, 错误 := Windows包.字符串目标SID("S-1-15-2-1")
	if 错误 != nil {
		return false
	}
	var 置顶 运行时包.固定器
	置顶.固定(SID)
	defer 置顶.解除固定()
	受托方 := Windows包.受托方{受托方表单: Windows包.受托方是SID, 受托方类型: Windows包.受托方是正确已知组, 受托方值: Windows包.受托方值来源SID(SID)}
	var 掩码 uint32
	进程 := Windows包.新建延迟系统动态链接库("advapi32.dll").新建进程("GetEffectiveRightsFromAclW")
	结果, _, _ := 进程.调用(uintptr(不安全操作包.指针(ACL)), uintptr(不安全操作包.指针(&受托方)), uintptr(不安全操作包.指针(&掩码)))
	运行时包.保留存活(安全描述符)
	if 结果 != 0 {
		return false
	}
	预期 := uint32(Windows包.文件通用读取 | Windows包.文件通用执行)
	return 掩码&预期 == 预期 || 掩码&Windows包.通用全部 != 0
}

// 回收只移除本次 SID 的 访问控制项，保留其他执行及用户在运行期间新增的权限。
func 移除沙箱访问控制项集(ACL *Windows包.ACL, SID *Windows包.SID) ([]byte, bool, error) {
	头部 := 不安全操作包.切片((*byte)(不安全操作包.指针(ACL)), 8)
	大小 := int(二进制包.小端字节序.无符号整数16(头部[2:4]))
	if 大小 < 8 {
		return nil, false, 工具包.无效错误
	}
	结果 := append([]byte(nil), 头部...)
	var 数量 uint16
	变化通知 := false
	for 索引 := uint32(0); 索引 < uint32(ACL.访问控制项数量); 索引++ {
		var 访问控制项 *Windows包.访问允许访问控制项
		if 错误 := Windows包.获取访问控制项(ACL, 索引, &访问控制项); 错误 != nil {
			return nil, false, 错误
		}
		if 访问控制项.头部.访问控制项大小 < 4 || int(访问控制项.头部.访问控制项大小) > 大小 {
			return nil, false, 工具包.无效错误
		}
		if (访问控制项.头部.访问控制项类型 == Windows包.访问允许访问控制项类型 || 访问控制项.头部.访问控制项类型 == Windows包.访问已拒绝访问控制项类型) && 访问控制项.头部.访问控制项大小 >= 16 {
			访问控制项SID := (*Windows包.SID)(不安全操作包.指针(&访问控制项.SID开始))
			if 访问控制项SID.是否有效() && 访问控制项SID.等于(SID) {
				变化通知 = true
				continue
			}
		}
		结果 = append(结果, 不安全操作包.切片((*byte)(不安全操作包.指针(访问控制项)), int(访问控制项.头部.访问控制项大小))...)
		数量++
	}
	二进制包.小端字节序.放入无符号整数16(结果[2:4], uint16(len(结果)))
	二进制包.小端字节序.放入无符号整数16(结果[4:6], 数量)
	return 结果, 变化通知, nil
}
```
