```go
// 包 运行时资源 校验随应用提供的 Python、节点 和 Git 资源，不下载资源或修改全局环境。
package 运行时资源

import (
	上下文包 "context"
	SHA256包 "crypto/sha256"
	十六进制包 "encoding/hex"
	JSON包 "encoding/json"
	错误判断包 "errors"
	格式化包 "fmt"
	输入输出包 "io"
	文件系统包 "io/fs"
	操作系统包 "os"
	路径包 "path"
	文件路径包 "path/filepath"
	字符串包 "strings"
	同步包 "sync"
	原子操作包 "sync/atomic"
)

// 格式版本 固定当前完整资源清单结构；旧缓存由调用方使用独立目录保留。
const 格式版本 = 3

const 最大清单字节 = 8 << 20
const 最大文件集 = 30000
const 最大载荷字节 int64 = 2 << 30

// 可执行文件 记录资源内入口；路径 是使用斜杠分隔的相对路径。
type 可执行文件 struct {
	版本 string `json:"version"`
	路径 string `json:"path"`
}

// Python包 记录离线组合进统一解释器的第三方依赖及其模块入口。
type Python包 struct {
	名称 string `json:"name"`
	版本 string `json:"version"`
	路径 string `json:"path"`
}

// 工作区运行时 固定技能共享依赖，解释器复用 资源清单 中的统一入口。
type 工作区运行时 struct {
	版本    string `json:"version"`
	节点模块集 string `json:"node_modules"`
	二进制目录 string `json:"bin_directory"`
}

// Git运行时 固定 Git 入口及其发行包内的辅助程序、模板和系统配置路径。
type Git运行时 struct {
	可执行文件
	进程执行路径 string `json:"exec_path"`
	模板路径   string `json:"template_path"`
	配置路径   string `json:"config_path"`
}

// 文件 记录普通文件的内容，或符号链接原文；不允许链接越出资源根目录。
type 文件 struct {
	路径     string `json:"path"`
	大小     int64  `json:"size,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	链接     string `json:"link,omitempty"`
}

// 资源清单 固定一个平台的完整运行时资源，准备命令生成后由应用只读使用。
type 资源清单 struct {
	格式版本     int       `json:"format_version"`
	平台       string    `json:"platform"`
	架构       string    `json:"arch"`
	Python   可执行文件     `json:"python"`
	节点       可执行文件     `json:"node"`
	Git      Git运行时    `json:"git"`
	Python包集 []Python包 `json:"python_packages,omitempty"`
	工作区      *工作区运行时   `json:"workspace,omitempty"`
	文件集      []文件      `json:"files"`
}

func 本地路径(名称 string) bool {
	return 名称 != "." && 文件系统包.有效路径(名称) && !字符串包.包含任一字符(名称, `\:`) && !字符串包.包含字符(名称, 0)
}

// 解码 严格限制清单大小、字段和尾随内容；具体文件与平台由 加载 复核。
func 解码(读取器 输入输出包.读取器) (资源清单, error) {
	var 值 资源清单
	数据, 错误 := 输入输出包.读取全部(输入输出包.上限读取器(读取器, 最大清单字节+1))
	if 错误 != nil || len(数据) > 最大清单字节 {
		return 值, 错误判断包.新建("运行时清单读取失败或超过限制")
	}
	解码器 := JSON包.新建解码器(字符串包.新建读取器(string(数据)))
	解码器.禁止未知字段集()
	if 错误 := 解码器.解码(&值); 错误 != nil {
		return 值, 格式化包.格式化创建错误("运行时清单无效: %w", 错误)
	}
	if 解码器.解码(new(any)) != 输入输出包.文件结束 || 值.格式版本 != 格式版本 || len(值.文件集) == 0 || len(值.文件集) > 最大文件集 {
		return 值, 错误判断包.新建("运行时清单格式或文件数量无效")
	}
	return 值, nil
}

func 哈希文件(执行上下文 上下文包.上下文, 根 *操作系统包.根, 名称 string) (_ string, 大小 int64, 返回错误 error) {
	文件, 错误 := 根.打开(文件路径包.来源斜杠(名称))
	if 错误 != nil {
		return "", 0, 错误
	}
	defer func() { 返回错误 = 错误判断包.合并(返回错误, 文件.关闭()) }()
	哈希 := SHA256包.新建()
	缓冲区 := make([]byte, 128<<10)
	for {
		if 错误 := 执行上下文.获取错误(); 错误 != nil {
			return "", 大小, 错误
		}
		数量, 错误 := 文件.读取(缓冲区)
		if 数量 > 0 {
			大小 += int64(数量)
			if 大小 > 最大载荷字节 {
				return "", 大小, 错误判断包.新建("运行时资源超过大小限制")
			}
			_, _ = 哈希.写入(缓冲区[:数量]) // 哈希.哈希.写入 不返回错误。
		}
		if 错误 == 输入输出包.文件结束 {
			return 十六进制包.编码目标字符串(哈希.校验和(nil)), 大小, nil
		}
		if 错误 != nil {
			return "", 大小, 错误
		}
	}
}

// 资源库存 计算已准备目录的完整文件清单，不包含根目录的 清单.JSON。
// 调用方必须保证准备目录不与任务执行共享，返回前不激活该目录。
func 资源库存(执行上下文 上下文包.上下文, 目录 string) (_ []文件, 返回错误 error) {
	根, 错误 := 操作系统包.打开根(目录)
	if 错误 != nil {
		return nil, 错误
	}
	defer func() { 返回错误 = 错误判断包.合并(返回错误, 根.关闭()) }()
	var 文件集 []文件
	错误 = 文件系统包.遍历目录(根.文件系统(), ".", func(名称 string, 条目 文件系统包.目录条目, 遍历错误 error) error {
		if 遍历错误 != nil {
			return 遍历错误
		}
		if 错误 := 执行上下文.获取错误(); 错误 != nil {
			return 错误
		}
		if 条目.是否目录() || 名称 == "manifest.json" {
			return nil
		}
		if !本地路径(名称) || len(文件集) >= 最大文件集 {
			return 错误判断包.新建("运行时文件路径或数量无效")
		}
		项 := 文件{路径: 名称}
		if 条目.类型()&操作系统包.模式符号链接 != 0 {
			项.链接, 错误 = 根.读取符号链接(文件路径包.来源斜杠(名称))
			if 错误 != nil || 文件路径包.是否绝对路径(项.链接) || !本地路径(路径包.合并(路径包.目录(名称), 文件路径包.目标斜杠(项.链接))) {
				return 错误判断包.新建("运行时链接越出资源目录")
			}
			// 打开根 校验完整链接链，包括中间目录链接。
			if _, 错误 := 根.状态信息(文件路径包.来源斜杠(名称)); 错误 != nil {
				return 格式化包.格式化创建错误("运行时链接不可用: %w", 错误)
			}
		} else if 条目.类型().是否普通文件() {
			// 目录顺序先固定，再由有界工作者计算内容摘要，避免完整依赖树的
			// 小文件安全打开与大型库读取串行占满启动时限。
		} else {
			return 错误判断包.新建("运行时资源包含特殊文件")
		}
		文件集 = append(文件集, 项)
		return nil
	})
	if 错误 != nil {
		return nil, 错误
	}
	检查集, 取消函数 := 上下文包.带有取消原因(执行上下文)
	defer 取消函数(nil)
	var 下一个, 总计 原子操作包.整数64
	var 工作线程集 同步包.等待组
	for range min(4, len(文件集)) {
		工作线程集.Go(func() {
			for 检查集.获取错误() == nil {
				索引 := 下一个.添加(1) - 1
				if 索引 >= int64(len(文件集)) {
					return
				}
				项 := &文件集[索引]
				if 项.链接 != "" {
					continue
				}
				var 错误 error
				项.SHA256, 项.大小, 错误 = 哈希文件(检查集, 根, 项.路径)
				if 错误 != nil {
					取消函数(错误)
					return
				}
				if 总计.添加(项.大小) > 最大载荷字节 {
					取消函数(错误判断包.新建("运行时资源总大小超过限制"))
					return
				}
			}
		})
	}
	// 返回和关闭 根 前等待所有校验工作者；失败会取消其他读取，不留下后台任务。
	工作线程集.等待()
	if 错误 := 上下文包.原因(检查集); 错误 != nil {
		return nil, 错误
	}
	return 文件集, nil
}
```
