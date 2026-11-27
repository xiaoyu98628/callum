```go
package 运行

import (
	上下文包 "context"
	SHA256包 "crypto/sha256"
	十六进制包 "encoding/hex"
	JSON包 "encoding/json"
	错误判断包 "errors"
	操作系统包 "os"
	文件路径包 "path/filepath"
	运行时包 "runtime"
	切片包 "slices"
	字符串包 "strings"
	UTF8包 "unicode/utf8"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

type 固定路径 struct {
	请求的, 实际 string
	文件      *操作系统包.文件
	信息      操作系统包.文件信息
	身份      string
}

type 固定命令 struct {
	保护                   工具包.受保护数据
	命令                   []string
	环境                   []string
	可执行文件, 目录, 工作区       *固定路径
	快照                   JSON包.原始JSON消息
	边界                   工具包.命令边界
	沙箱可执行文件, 临时父级, 受保护目录 *固定路径
	临时路径, 配置档            string
	启动参数                 []string
	沙箱状态                 沙箱状态
	读取仅目录集               []*固定路径
	工作目录                 *固定路径
}

func 清理目录(路径 string) (string, error) {
	if !UTF8包.有效字符串(路径) || 字符串包.包含字符(路径, 0) || (!文件路径包.是否绝对路径(路径) && !文件路径包.是否本地路径(路径)) {
		return "", 工具包.无效错误
	}
	return 文件路径包.清理(路径), nil
}

func 校验可执行文件路径(路径 string) error {
	if 运行时包.目标操作系统 == "windows" && (字符串包.存在前缀(路径, `\\?\`) || 字符串包.存在前缀(路径, `\\.\`) || (文件路径包.卷名称(路径) != "" && !文件路径包.是否绝对路径(路径)) || 字符串包.包含(字符串包.裁剪前缀(路径, 文件路径包.卷名称(路径)), ":")) {
		return 工具包.无效错误
	}
	return nil
}

func 固定目标路径(路径 string, 目录 bool) (_ *固定路径, 返回错误 error) {
	实际, 错误 := 文件路径包.求值符号链接集(路径)
	if 错误 != nil {
		return nil, 工具包.不可用错误
	}
	信息, 错误 := 操作系统包.状态信息(实际)
	if 错误 != nil || (目录 && !信息.是否目录()) || (!目录 && !信息.模式().是否普通文件()) {
		return nil, 工具包.不可用错误
	}
	文件, 错误 := 打开目标路径(实际)
	if 错误 != nil {
		return nil, 工具包.不可用错误
	}
	defer func() {
		if 返回错误 != nil {
			返回错误 = 错误判断包.合并(返回错误, 文件.关闭())
		}
	}()
	已打开, 错误 := 文件.状态信息()
	if 错误 != nil || !操作系统包.相同文件(信息, 已打开) {
		return nil, 工具包.已拒绝错误
	}
	身份, 错误 := 文件身份(文件, 已打开)
	if 错误 != nil {
		return nil, 错误
	}
	return &固定路径{请求的: 路径, 实际: 实际, 文件: 文件, 信息: 已打开, 身份: 身份}, nil
}

func (当前固定路径 *固定路径) 检查(可执行文件 bool) error {
	实际, 错误 := 文件路径包.求值符号链接集(当前固定路径.请求的)
	if 错误 != nil || 实际 != 当前固定路径.实际 {
		return 工具包.已拒绝错误
	}
	当前, 错误 := 操作系统包.状态信息(实际)
	if 错误 != nil || !操作系统包.相同文件(当前, 当前固定路径.信息) {
		return 工具包.已拒绝错误
	}
	已打开, 错误 := 当前固定路径.文件.状态信息()
	if 错误 != nil || !操作系统包.相同文件(已打开, 当前固定路径.信息) {
		return 工具包.已拒绝错误
	}
	if 可执行文件 && (已打开.大小() != 当前固定路径.信息.大小() || 已打开.模式() != 当前固定路径.信息.模式() || !已打开.模块时间().相等(当前固定路径.信息.模块时间())) {
		return 工具包.已拒绝错误
	}
	return nil
}

func 准备模块(执行上下文 上下文包.上下文, 范围 工具包.范围, 当前参数 参数, 环境 []string, 权限 工具包.执行权限, 保护 工具包.受保护数据) (_ 工具包.工具调用, 返回错误 error) {
	if 错误 := 执行上下文.获取错误(); 错误 != nil {
		return nil, 错误
	}
	if 运行时包.目标操作系统 != "darwin" && 运行时包.目标操作系统 != "linux" && 运行时包.目标操作系统 != "windows" {
		return nil, 工具包.不可用错误
	}
	固定 := &固定命令{命令: 当前参数.命令, 保护: 保护, 环境: 切片包.复制(环境)}
	defer func() {
		if 返回错误 != nil {
			返回错误 = 错误判断包.合并(返回错误, 固定.close())
		}
	}()
	if 范围 != (工具包.范围{}) {
		if 范围.标识 == "" || !文件路径包.是否绝对路径(范围.根路径) {
			return nil, 工具包.已拒绝错误
		}
		var 错误 error
		固定.工作区, 错误 = 固定目标路径(范围.根路径, true)
		if 错误 != nil {
			return nil, 错误
		}
	}
	工作目录 := 当前参数.工作目录
	if 工作目录 == "" {
		工作目录 = 范围.根路径
	}
	if !文件路径包.是否绝对路径(工作目录) {
		if 范围.标识 == "" {
			return nil, 工具包.路径基准错误
		}
		工作目录 = 文件路径包.合并(范围.根路径, 工作目录)
	}
	if 工作目录 == "" {
		return nil, 工具包.路径基准错误
	}
	var 错误 error
	固定.目录, 错误 = 固定目标路径(工作目录, true)
	if 错误 != nil {
		return nil, 错误
	}
	程序, 错误 := 解析可执行文件(当前参数.命令[0], 固定.目录.实际, 环境)
	if 错误 != nil {
		return nil, 错误
	}
	固定.可执行文件, 错误 = 固定目标路径(程序, false)
	if 错误 != nil {
		return nil, 错误
	}
	固定.命令[0] = 固定.可执行文件.实际
	if 权限.工作目录 != "" {
		if !文件路径包.是否绝对路径(权限.工作目录) {
			return nil, 工具包.已拒绝错误
		}
		固定.工作目录, 错误 = 固定目标路径(权限.工作目录, true)
		if 错误 != nil {
			return nil, 错误
		}
	}
	固定.环境 = append(固定.环境, "PWD="+固定.目录.实际)
	for _, 目录 := range 保护.只读目录集 {
		if !文件路径包.是否绝对路径(目录) {
			return nil, 工具包.已拒绝错误
		}
		根, 错误 := 固定目标路径(目录, true)
		if 错误 != nil {
			return nil, 错误
		}
		// 装配层传入真实路径。拒绝后续把只读根替换为链接而扩大应用数据读取例外。
		if 根.实际 != 文件路径包.清理(目录) {
			return nil, 错误判断包.合并(工具包.已拒绝错误, 根.文件.关闭())
		}
		固定.读取仅目录集 = append(固定.读取仅目录集, 根)
	}
	if 错误 := 固定.配置沙箱(当前参数, 权限, 保护); 错误 != nil {
		return nil, 错误
	}
	摘要 := SHA256包.新建()
	键集 := make([]string, 0, len(固定.环境))
	for _, 条目 := range 固定.环境 {
		键, _, _ := 字符串包.截断(条目, "=")
		键集 = append(键集, 键)
		// 私有临时目录的随机名称不扩大操作范围，不能改变同一命令的拒绝/未知结果身份。
		if 固定.临时路径 != "" {
			条目 = 字符串包.替换全部(条目, 固定.临时路径, ":command_temp")
		}
		摘要.写入([]byte(条目))
		摘要.写入([]byte{0})
	}
	详情 := struct {
		可执行文件     string   `json:"executable"`
		可执行文件身份   string   `json:"executable_identity"`
		可执行文件大小   int64    `json:"executable_size"`
		可执行文件已修改  int64    `json:"executable_modified_ns"`
		工作目录      string   `json:"cwd"`
		工作目录身份    string   `json:"cwd_identity"`
		环境配置档     string   `json:"environment_profile"`
		环境键集      []string `json:"environment_keys"`
		环境SHA256  string   `json:"environment_sha256"`
		边界        工具包.命令边界 `json:"execution_boundary"`
		沙箱内       bool     `json:"sandboxed"`
		沙箱后端      string   `json:"sandbox_backend,omitempty"`
		可写根目录集    []string `json:"writable_roots"`
		可读根目录集    []string `json:"readable_roots,omitempty"`
		只读根目录集    []string `json:"read_only_roots,omitempty"`
		执行工作目录    string   `json:"run_work_directory,omitempty"`
		临时目录策略    string   `json:"temporary_directory_policy,omitempty"`
		网络访问      bool     `json:"network_access"`
		理由        string   `json:"justification,omitempty"`
		配置档SHA256 string   `json:"profile_sha256,omitempty"`
	}{可执行文件: 固定.可执行文件.实际, 可执行文件身份: 固定.可执行文件.身份, 可执行文件大小: 固定.可执行文件.信息.大小(), 可执行文件已修改: 固定.可执行文件.信息.模块时间().Unix纳秒(), 工作目录: 固定.目录.实际, 工作目录身份: 固定.目录.身份, 环境配置档: "base-v1", 环境键集: 键集, 环境SHA256: 十六进制包.编码目标字符串(摘要.校验和(nil)), 边界: 固定.边界, 沙箱内: 固定.边界.沙箱内, 网络访问: !固定.边界.沙箱内, 理由: 当前参数.理由, 可写根目录集: []string{}}
	for _, 根 := range 固定.读取仅目录集 {
		详情.只读根目录集 = append(详情.只读根目录集, 根.实际)
	}
	if 固定.边界.沙箱内 {
		详情.环境配置档 = "base-v2-sandbox"
		详情.沙箱后端 = 固定.沙箱后端()
		详情.可读根目录集 = 固定.沙箱状态.可读根目录集()
		详情.临时目录策略 = "private-per-command"
		if 固定.工作目录 != nil {
			详情.执行工作目录 = 固定.工作目录.实际
			详情.临时目录策略 = "private-per-command-and-run"
		}
		if 固定.工作区 != nil && 固定.边界.模式 == 工具包.沙箱工作区写入 {
			详情.可写根目录集 = append(详情.可写根目录集, 固定.工作区.实际)
		}
		摘要 := SHA256包.校验和256([]byte(固定.配置档))
		详情.配置档SHA256 = 十六进制包.编码目标字符串(摘要[:])
	}
	固定.快照, 错误 = JSON包.序列化(详情)
	if 错误 != nil {
		return nil, 错误
	}
	if len(固定.快照) > 工具包.最大参数字节 {
		return nil, 工具包.超出上限错误
	}
	if 错误 := 固定.检查(执行上下文); 错误 != nil {
		return nil, 错误
	}
	return &工具调用{固定: 固定}, nil
}

func (当前固定命令 *固定命令) 检查(执行上下文 上下文包.上下文) error {
	if 错误 := 执行上下文.获取错误(); 错误 != nil {
		return 错误
	}
	if 错误 := 当前固定命令.沙箱状态.检查(执行上下文); 错误 != nil {
		return 错误
	}
	if 当前固定命令.工作区 != nil {
		if 错误 := 当前固定命令.工作区.检查(false); 错误 != nil {
			return 错误
		}
	}
	for _, 根 := range 当前固定命令.读取仅目录集 {
		if 错误 := 根.检查(false); 错误 != nil {
			return 错误
		}
	}
	for _, 路径 := range []*固定路径{当前固定命令.沙箱可执行文件, 当前固定命令.临时父级, 当前固定命令.受保护目录, 当前固定命令.工作目录} {
		if 路径 != nil {
			if 错误 := 路径.检查(路径 == 当前固定命令.沙箱可执行文件); 错误 != nil {
				return 错误
			}
		}
	}
	if 当前固定命令.边界.沙箱内 {
		if 错误 := 检查SQLite链接集(当前固定命令.保护); 错误 != nil {
			return 错误
		}
	}
	if 错误 := 当前固定命令.目录.检查(false); 错误 != nil {
		return 错误
	}
	return 当前固定命令.可执行文件.检查(true)
}

func (当前固定命令 *固定命令) close() error {
	错误 := 当前固定命令.沙箱状态.close()
	for _, 路径 := range append([]*固定路径{当前固定命令.可执行文件, 当前固定命令.目录, 当前固定命令.工作区, 当前固定命令.沙箱可执行文件, 当前固定命令.临时父级, 当前固定命令.受保护目录, 当前固定命令.工作目录}, 当前固定命令.读取仅目录集...) {
		if 路径 != nil {
			错误 = 错误判断包.合并(错误, 路径.文件.关闭())
		}
	}
	return 错误
}

func 解析可执行文件(名称, 工作目录 string, 环境 []string) (string, error) {
	var 候选集 []string
	if 文件路径包.是否绝对路径(名称) {
		候选集 = []string{名称}
	} else if 字符串包.包含任一字符(名称, `/\`) {
		候选集 = []string{文件路径包.合并(工作目录, 名称)}
	} else {
		for _, 目录 := range 文件路径包.拆分列表(环境值(环境, "PATH")) {
			if 文件路径包.是否绝对路径(目录) {
				候选集 = append(候选集, 文件路径包.合并(目录, 名称))
			}
		}
	}
	for _, 候选 := range 候选集 {
		名称集 := []string{候选}
		if 运行时包.目标操作系统 == "windows" && 文件路径包.扩展名(候选) == "" {
			名称集 = []string{候选 + ".exe", 候选 + ".com"}
		}
		for _, 路径 := range 名称集 {
			if 运行时包.目标操作系统 == "windows" {
				扩展名 := 字符串包.目标小写(文件路径包.扩展名(路径))
				if 扩展名 != ".exe" && 扩展名 != ".com" {
					continue
				}
			}
			信息, 错误 := 操作系统包.状态信息(路径)
			if 错误 == nil && 信息.模式().是否普通文件() && (运行时包.目标操作系统 == "windows" || 信息.模式().权限()&0111 != 0) {
				return 路径, nil
			}
		}
	}
	return "", 工具包.不可用错误
}
```
