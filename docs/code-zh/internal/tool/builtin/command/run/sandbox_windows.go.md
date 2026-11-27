```go
package 运行

import (
	上下文包 "context"
	随机包 "crypto/rand"
	JSON包 "encoding/json"
	错误判断包 "errors"
	格式化包 "fmt"
	文件系统包 "io/fs"
	操作系统包 "os"
	文件路径包 "path/filepath"
	切片包 "slices"
	字符串包 "strings"
	Unicode包 "unicode"

	Windows包 "golang.org/x/sys/windows"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

const 沙箱恢复日志版本 = 1
const 最大沙箱条目集 = 65536
const 最大沙箱恢复日志字节 = 16 << 20
const 最大沙箱根目录集 = 4 // 工作目录、程序目录、可选工作区及完整内置运行时树。

type 沙箱根 struct {
	路径 string `json:"path"`
	身份 string `json:"identity"`
	写入 bool   `json:"write"`
	环境 bool   `json:"ambient,omitempty"`
}

type 沙箱ACL条目 struct {
	路径  string `json:"path"`
	身份  string `json:"identity"`
	目录  bool   `json:"directory"`
	掩码  uint32 `json:"mask"`
	拒绝  bool   `json:"deny,omitempty"`
	自身仅 bool   `json:"self_only,omitempty"` // 祖先目录的删除保护不向普通子项传播。
}

type 沙箱恢复日志 struct {
	受保护目录 string    `json:"protected_directory"`
	版本    int       `json:"version"`
	名称    string    `json:"name"`
	SID   string    `json:"sid"`
	临时    string    `json:"temporary"`
	根目录集  []沙箱根     `json:"roots"`
	条目集   []沙箱ACL条目 `json:"entries"`
}

type 沙箱状态 struct {
	根目录集   []*固定路径
	恢复日志   沙箱恢复日志
	恢复日志路径 string
	SID    *Windows包.SID
	词元     Windows包.词元
	活动     bool
	配置档自有  bool
	ACL已触及 bool
}

func (当前沙箱状态 *沙箱状态) 可读根目录集() []string {
	var 根目录集 []string
	for _, 根 := range 当前沙箱状态.恢复日志.根目录集 {
		根目录集 = append(根目录集, 根.路径)
	}
	return 根目录集
}

func (当前沙箱状态 *沙箱状态) 检查(执行上下文 上下文包.上下文) error {
	if 错误 := 执行上下文.获取错误(); 错误 != nil {
		return 错误
	}
	for _, 根 := range 当前沙箱状态.根目录集 {
		if 错误 := 根.检查(false); 错误 != nil {
			return 错误
		}
	}
	return nil
}

func (当前沙箱状态 *沙箱状态) close() error {
	var 错误 error
	if 当前沙箱状态.活动 || 当前沙箱状态.词元 != 0 {
		错误 = 当前沙箱状态.清理()
	}
	for _, 根 := range 当前沙箱状态.根目录集 {
		错误 = 错误判断包.合并(错误, 根.文件.关闭())
	}
	当前沙箱状态.根目录集 = nil
	return 错误
}

func (*固定命令) 沙箱后端() string { return "windows-appcontainer" }

func (当前固定命令 *固定命令) 配置沙箱(当前参数 参数, 权限 工具包.执行权限, 保护 工具包.受保护数据) error {
	项目写入, 已处理, 错误 := 当前固定命令.沙箱请求(当前参数, 权限)
	if 错误 != nil || 已处理 {
		return 错误
	}
	if 保护.目录 == "" {
		return 格式化包.格式化创建错误("Windows 沙箱需要持有应用数据目录的恢复记录: %w", 工具包.不可用错误)
	}
	for _, 路径 := range []string{当前固定命令.目录.实际, 当前固定命令.可执行文件.实际, 保护.目录, 操作系统包.创建临时目录()} {
		卷 := 文件路径包.卷名称(路径)
		if len(卷) != 2 || 卷[1] != ':' {
			return 格式化包.格式化创建错误("Windows 受限命令需要本地卷路径: %w", 工具包.不可用错误)
		}
	}
	for _, 进程 := range []*Windows包.延迟进程{创建应用容器配置档, 删除应用容器配置档, 派生应用容器SID, 创建受限词元} {
		if 错误 := 进程.查找(); 错误 != nil {
			return 错误判断包.合并(工具包.不可用错误, 错误)
		}
	}
	当前固定命令.受保护目录, 错误 = 固定目标路径(保护.目录, true)
	if 错误 != nil {
		return 错误
	}
	保护.目录 = 当前固定命令.受保护目录.实际
	当前固定命令.保护.目录 = 保护.目录
	当前固定命令.临时父级, 错误 = 固定目标路径(操作系统包.创建临时目录(), true)
	if 错误 != nil {
		return 错误
	}
	if 保护.包含(当前固定命令.临时父级.实际) || (项目写入 && 保护.包含(当前固定命令.工作区.实际)) {
		return 工具包.已拒绝错误
	}
	当前固定命令.临时路径 = 文件路径包.合并(当前固定命令.临时父级.实际, "callum-command-"+随机包.文本())
	当前固定命令.边界.沙箱内, 当前固定命令.边界.模式 = true, 工具包.沙箱只读
	if 项目写入 {
		当前固定命令.边界.模式 = 工具包.沙箱工作区写入
		当前固定命令.边界.创建工作区目录 = 当前固定命令.创建工作区目录()
	}
	当前状态 := &当前固定命令.沙箱状态
	当前状态.恢复日志 = 沙箱恢复日志{版本: 沙箱恢复日志版本, 名称: "callum.command." + 随机包.文本(), 临时: 当前固定命令.临时路径, 受保护目录: 当前固定命令.受保护目录.实际}
	路径集 := []string{当前固定命令.目录.实际, 文件路径包.目录(当前固定命令.可执行文件.实际)}
	if 当前固定命令.工作目录 != nil {
		路径集 = append(路径集, 当前固定命令.工作目录.实际)
	}
	for _, 根 := range 当前固定命令.读取仅目录集 {
		路径集 = append(路径集, 根.实际)
	}
	if 当前固定命令.工作区 != nil {
		路径集 = append(路径集, 当前固定命令.工作区.实际)
	}
	for _, 路径 := range 路径集 {
		if 切片包.包含函数(当前状态.根目录集, func(根 *固定路径) bool { return 字符串包.相等折叠(根.实际, 路径) }) {
			continue
		}
		if 保护.读取受保护(路径) {
			return 工具包.已拒绝错误
		}
		根, 错误 := 固定目标路径(路径, true)
		if 错误 != nil {
			return 错误
		}
		当前状态.根目录集 = append(当前状态.根目录集, 根)
		写入 := 项目写入 && 当前固定命令.工作区 != nil && 字符串包.相等折叠(根.实际, 当前固定命令.工作区.实际)
		写入 = 写入 || (当前固定命令.工作目录 != nil && 字符串包.相等折叠(根.实际, 当前固定命令.工作目录.实际))
		if 写入 && 是Windows系统目录(根.实际, 当前固定命令.环境) {
			return 工具包.已拒绝错误
		}
		环境 := !写入 && (是Windows系统目录(根.实际, 当前固定命令.环境) || 环境Windows读取(根.实际))
		当前状态.恢复日志.根目录集 = append(当前状态.恢复日志.根目录集, 沙箱根{路径: 根.实际, 身份: 根.身份, 写入: 写入, 环境: 环境})
	}
	if len(当前状态.恢复日志.根目录集) > 最大沙箱根目录集 {
		return 工具包.超出上限错误
	}
	配置档, 错误 := JSON包.序列化(struct {
		策略   string
		根目录集 []沙箱根
	}{策略: "windows-appcontainer-v1;no-network;write-restricted;private-temp", 根目录集: 当前状态.恢复日志.根目录集})
	if 错误 != nil {
		return 错误
	}
	当前固定命令.配置档 = string(配置档)
	// 随机私有目录不进入权限指纹；实际读写根目录由快照绑定。
	当前固定命令.环境 = 沙箱环境(当前固定命令.环境, 当前固定命令.临时路径)
	return nil
}

func (当前固定命令 *固定命令) 初始化执行(执行上下文 上下文包.上下文) (清理 func() error, 返回错误 error) {
	if !当前固定命令.边界.沙箱内 {
		return func() error { return nil }, nil
	}
	当前状态 := &当前固定命令.沙箱状态
	if 当前状态.活动 {
		return nil, 工具包.已拒绝错误
	}
	// 恢复目录位于模型不可访问的应用数据目录内。每次执行拥有独立 SID 与日志。
	目录 := 文件路径包.合并(当前固定命令.受保护目录.实际, "command-sandboxes")
	if 错误 := 确保沙箱恢复日志目录(目录); 错误 != nil {
		return nil, 错误
	}
	当前状态.恢复日志路径 = 文件路径包.合并(目录, 当前状态.恢复日志.名称+".json")
	SID, 错误 := 沙箱SID(当前状态.恢复日志.名称)
	if 错误 != nil {
		return nil, 错误
	}
	当前状态.SID, 当前状态.恢复日志.SID = SID, SID.字符串()
	if 错误 := 操作系统包.创建目录(当前固定命令.临时路径, 0700); 错误 != nil {
		return nil, 错误
	}
	defer func() {
		if 返回错误 != nil {
			清理错误 := 当前状态.清理()
			if 清理错误 != nil {
				返回错误 = 错误判断包.合并(返回错误, 工具包.未知操作结果错误, 清理错误)
			}
		}
	}()
	if 错误 := 安全新建沙箱目录(当前固定命令.临时路径); 错误 != nil {
		return nil, 错误
	}
	if 错误 := 当前固定命令.收集沙箱访问控制目录列表(执行上下文); 错误 != nil {
		return nil, 错误
	}
	if 错误 := 写入沙箱恢复日志(当前状态.恢复日志路径, 当前状态.恢复日志); 错误 != nil {
		return nil, 错误
	}
	当前状态.活动 = true
	当前状态.配置档自有, 错误 = 新建沙箱配置档(当前状态.恢复日志.名称, SID)
	if 错误 != nil {
		return nil, 错误
	}
	当前状态.词元, 错误 = 沙箱受限词元(SID)
	if 错误 != nil {
		return nil, 错误
	}
	if 错误 := 带有沙箱ACL加锁(func() error {
		当前状态.ACL已触及 = true
		for _, 条目 := range 当前状态.恢复日志.条目集 {
			if 错误 := 执行上下文.获取错误(); 错误 != nil {
				return 错误
			}
			if 错误 := 更新沙箱ACL(条目, SID, false); 错误 != nil {
				return 错误
			}
		}
		return nil
	}); 错误 != nil {
		return nil, 错误
	}
	return 当前状态.清理, nil
}

func (当前固定命令 *固定命令) 收集沙箱访问控制目录列表(执行上下文 上下文包.上下文) error {
	当前状态 := &当前固定命令.沙箱状态
	条目集 := map[string]沙箱ACL条目{}
	收集 := func(根 string, 写入, 内容块 bool) error {
		return 文件路径包.遍历目录(根, func(路径 string, 条目 文件系统包.目录条目, 遍历错误 error) error {
			if 错误 := 执行上下文.获取错误(); 错误 != nil {
				return 错误
			}
			if 遍历错误 != nil {
				return 遍历错误
			}
			if 条目.类型()&操作系统包.模式符号链接 != 0 {
				if 条目.是否目录() {
					return 文件路径包.跳过目录
				}
				return nil
			}
			if 内容块 && !当前固定命令.保护.读取受保护(路径) {
				return 文件路径包.跳过目录
			}
			// 应用数据保留独立拒绝条目，遍历不会进入其中。
			受保护 := 当前固定命令.保护.读取受保护(路径)
			if !内容块 && !字符串包.相等折叠(路径, 根) && 受保护 {
				return 文件路径包.跳过目录
			}
			if !内容块 && 受保护 {
				return 工具包.已拒绝错误
			}
			信息, 错误 := 条目.获取信息()
			if 错误 != nil {
				return 错误
			}
			if !信息.是否目录() && !信息.模式().是否普通文件() {
				return 工具包.已拒绝错误
			}
			掩码 := uint32(Windows包.文件通用读取 | Windows包.文件通用执行)
			拒绝 := 内容块 || 受保护沙箱路径(路径) || 当前固定命令.保护.只读包含(路径)
			if 写入 && !拒绝 {
				掩码 |= Windows包.文件通用写入
				// 应用数据的祖先目录不能被重命名或删除。
				if !字符串包.相等折叠(路径, 根) && !当前固定命令.保护.保护后代(路径) {
					掩码 |= Windows包.删除
				}
			}
			if 拒绝 {
				掩码 = Windows包.文件写入数据 | Windows包.文件追加数据 | Windows包.文件写入扩展属性 | Windows包.文件写入属性集 | Windows包.删除 | Windows包.写入自主访问控制 | Windows包.写入所属对象 | 沙箱删除子级
			}
			if 内容块 {
				掩码 = 沙箱文件全部访问
			}
			身份, 错误 := 沙箱路径身份(路径)
			if 错误 != nil {
				return 错误
			}
			键 := 字符串包.目标小写(路径)
			旧, 存在 := 条目集[键]
			if 存在 && !拒绝 {
				掩码 |= 旧.掩码
			}
			自身仅 := 内容块 && 信息.是否目录() && 当前固定命令.保护.保护后代(路径)
			if 自身仅 {
				// 应用数据根目录保留禁止列举等权限，允许路径穿越至单独登记的 技能 子树；拒绝不向该子树继承。
				掩码 &^= Windows包.文件遍历
			}
			条目集[键] = 沙箱ACL条目{路径: 路径, 身份: 身份, 目录: 信息.是否目录(), 掩码: 掩码, 拒绝: 拒绝, 自身仅: 自身仅}
			if !内容块 && 当前固定命令.保护.只读包含(路径) {
				// 只读资源同时需要读取授权和独立写拒绝，不能只创建拒绝条目。
				条目集[键+"\x00readonly-read"] = 沙箱ACL条目{路径: 路径, 身份: 身份, 目录: 信息.是否目录(), 掩码: Windows包.文件通用读取 | Windows包.文件通用执行}
			}
			if !拒绝 && 信息.是否目录() && 当前固定命令.保护.保护后代(路径) {
				// 工作区父目录的继承 删除 授权不能抵消资源祖先的固定路径保护。
				条目集[键+"\x00ancestor"] = 沙箱ACL条目{路径: 路径, 身份: 身份, 目录: true, 掩码: Windows包.删除 | 沙箱删除子级, 拒绝: true, 自身仅: true}
			}
			if len(条目集) > 最大沙箱条目集 {
				return 工具包.超出上限错误
			}
			return nil
		})
	}
	for _, 根 := range 当前状态.恢复日志.根目录集 {
		// 系统目录沿用 Windows 已有 应用容器 读取授权；用户安装的程序目录显式授权。
		if 根.环境 {
			continue
		}
		if 错误 := 收集(根.路径, 根.写入, false); 错误 != nil {
			return 错误
		}
	}
	if 错误 := 收集(当前固定命令.临时路径, true, false); 错误 != nil {
		return 错误
	}
	if 错误 := 收集(当前固定命令.受保护目录.实际, false, true); 错误 != nil {
		return 错误
	}
	for _, 条目 := range 条目集 {
		当前状态.恢复日志.条目集 = append(当前状态.恢复日志.条目集, 条目)
	}
	// 拒绝先于授权建立。系统 API 不向已有子对象自动传播 ACL，避免改写符号链接目标。
	切片包.排序函数(当前状态.恢复日志.条目集, func(当前沙箱ACL条目, 当前沙箱ACL条目 沙箱ACL条目) int {
		if 当前沙箱ACL条目.拒绝 != 当前沙箱ACL条目.拒绝 {
			if 当前沙箱ACL条目.拒绝 {
				return -1
			}
			return 1
		}
		return 字符串包.比较(当前沙箱ACL条目.路径, 当前沙箱ACL条目.路径)
	})
	return nil
}

func 受保护沙箱路径(路径 string) bool {
	for 部分 := range 字符串包.拆分序号(文件路径包.目标斜杠(路径), "/") {
		if 切片包.包含([]string{".git", ".codex", ".agents", ".aws"}, 字符串包.目标小写(部分)) {
			return true
		}
	}
	return false
}

func 是Windows系统目录(路径 string, _ []string) bool {
	根, 错误 := Windows包.获取Windows目录()
	if 错误 != nil {
		return false
	}
	相对, 错误 := 文件路径包.相对路径(字符串包.目标小写(根), 字符串包.目标小写(路径))
	return 错误 == nil && 文件路径包.是否本地路径(相对)
}

// 仅系统 命令 /丁 /丙 创建目录 加单个普通名称可进入单层目录创建审核。
func (当前固定命令 *固定命令) 创建工作区目录() bool {
	if 当前固定命令.工作区 == nil || len(当前固定命令.命令) != 5 || !字符串包.相等折叠(当前固定命令.命令[1], "/d") || !字符串包.相等折叠(当前固定命令.命令[2], "/c") || !字符串包.相等折叠(当前固定命令.命令[3], "mkdir") {
		return false
	}
	名称 := 当前固定命令.命令[4]
	if !文件路径包.是否本地路径(名称) || 名称 == "" || 字符串包.存在前缀(名称, "-") {
		return false
	}
	for _, 当前读取器 := range 名称 {
		if !Unicode包.是字母(当前读取器) && !Unicode包.是数字(当前读取器) && 当前读取器 != '_' && 当前读取器 != '-' {
			return false
		}
	}
	系统根, 错误 := Windows包.获取Windows目录()
	if 错误 != nil {
		return false
	}
	实际, 解析错误 := 文件路径包.求值符号链接集(文件路径包.合并(系统根, "System32", "cmd.exe"))
	可信 := 解析错误 == nil && 字符串包.相等折叠(实际, 当前固定命令.可执行文件.实际)
	相对, 错误 := 文件路径包.相对路径(字符串包.目标小写(当前固定命令.工作区.实际), 字符串包.目标小写(当前固定命令.目录.实际))
	return 可信 && 错误 == nil && 文件路径包.是否本地路径(相对)
}
```
