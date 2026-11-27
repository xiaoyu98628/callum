```go
// 包 补丁 提供带版本校验和固定目标审批的文件补丁工具。
package 补丁

import (
	上下文包 "context"
	十六进制包 "encoding/hex"
	JSON包 "encoding/json"
	错误判断包 "errors"
	格式化包 "fmt"
	文件路径包 "path/filepath"
	运行时包 "runtime"
	切片包 "slices"
	字符串包 "strings"
	同步包 "sync"
	UTF8包 "unicode/utf8"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	工具参数包 "github.com/xiaoyu98628/callum/internal/tool/builtin/internal/arguments"
)

type 补丁版本 struct {
	路径   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type 补丁参数 struct {
	补丁        string         `json:"patch"`
	基准版本集 []补丁版本 `json:"base_versions"`
}

// 补丁工具 的互斥范围仅覆盖正在写入的重叠目标，不占用审批等待时间。
// 锁由工具实例持有，无后台任务；执行结束立即删除活动项。
type 补丁工具 struct {
	互斥锁         同步包.互斥锁
	活动     []*补丁调用
	变化通知    chan struct{}
	保护 工具包.受保护数据
}

// 新建 返回独立的补丁工具，重叠目标的写入互斥仅在该实例内共享。
func 新建(保护 工具包.受保护数据) 工具包.工具 {
	return &补丁工具{变化通知: make(chan struct{}), 保护: 保护}
}
func (*补丁工具) 获取定义() 工具包.定义 {
	return 工具包.定义{名称: "apply_patch", 说明: 格式化包.格式化字符串("创建或修改 UTF-8 文本文件，最多8个文件。整次调用的 JSON 参数最多 %d 字节，大文件通过脚本读取本地源文件生成，补丁按容量拆成多个小批次。patch 使用 *** Begin Patch / *** Add File: path（正文每行以+开始）/ *** Update File: path（@@ 后使用空格上下文、-旧行、+新行）/ *** End Patch。修改前用 read_file 获取整个文件的 sha256，在 base_versions 中为每个 Update File 提供同一路径和 sha256；新增文件不提供版本且不能覆盖已有文件。不支持删除、移动或模糊匹配。相对路径需要工作区，绝对路径不要求工作区。", 工具包.最大参数字节), 参数: JSON包.原始JSON消息(`{"type":"object","properties":{"patch":{"type":"string"},"base_versions":{"type":"array","items":{"type":"object","properties":{"path":{"type":"string"},"sha256":{"type":"string"}},"required":["path","sha256"],"additionalProperties":false}}},"required":["patch","base_versions"],"additionalProperties":false}`), 来源: "builtin", 版本: "2", 影响集: []工具包.影响{工具包.影响写入}}
}

func 补丁路径(路径 string) (string, error) {
	if 路径 == "" || len(路径) > 1024 || !UTF8包.有效字符串(路径) || 字符串包.包含任一字符(路径, "\x00\r\n") || (!文件路径包.是否绝对路径(路径) && !文件路径包.是否本地路径(路径)) {
		return "", 工具包.无效错误
	}
	if 运行时包.目标操作系统 == "windows" {
		// 不接受设备命名空间、备用数据流或驱动器相对路径。
		if 字符串包.存在前缀(路径, `\\?\`) || 字符串包.存在前缀(路径, `\\.\`) || 字符串包.包含(字符串包.裁剪前缀(路径, 文件路径包.卷名称(路径)), ":") {
			return "", 工具包.无效错误
		}
	}
	return 文件路径包.清理(路径), nil
}

func 解码补丁(原始 JSON包.原始JSON消息) (补丁参数, []补丁文件, error) {
	var 参数 补丁参数
	if _, 错误 := 工具参数包.解码(原始, &参数); 错误 != nil {
		return 参数, nil, 错误
	}
	if 参数.基准版本集 == nil {
		return 参数, nil, 工具包.无效错误
	}
	文件集, 错误 := 解析补丁(参数.补丁)
	if 错误 != nil {
		return 参数, nil, 错误
	}
	版本集 := map[string]string{}
	for 索引, 值 := range 参数.基准版本集 {
		路径, 错误 := 补丁路径(值.路径)
		值, 哈希错误 := 十六进制包.解码字符串(值.SHA256)
		if 错误 != nil || 哈希错误 != nil || len(值) != 32 || 版本集[路径] != "" {
			return 参数, nil, 工具包.无效错误
		}
		参数.基准版本集[索引] = 补丁版本{路径, 十六进制包.编码目标字符串(值)}
		版本集[路径] = 参数.基准版本集[索引].SHA256
	}
	for _, 文件 := range 文件集 {
		if 文件.动作 == "update" {
			if 版本集[文件.路径] == "" {
				return 参数, nil, 工具包.无效错误
			}
			delete(版本集, 文件.路径)
		}
	}
	if len(版本集) != 0 {
		return 参数, nil, 工具包.无效错误
	}
	切片包.排序函数(参数.基准版本集, func(当前补丁版本, 当前补丁版本 补丁版本) int { return 字符串包.比较(当前补丁版本.路径, 当前补丁版本.路径) })
	参数.补丁 = 规范化补丁(文件集)
	return 参数, 文件集, nil
}

func (*补丁工具) 校验(原始 JSON包.原始JSON消息) (JSON包.原始JSON消息, error) {
	参数, _, 错误 := 解码补丁(原始)
	if 错误 != nil {
		return nil, 错误
	}
	已规范化, 错误 := JSON包.序列化(参数)
	if len(已规范化) > 工具包.最大参数字节 {
		return nil, 工具包.超出上限错误
	}
	return 已规范化, 错误
}
func (当前补丁工具 *补丁工具) 准备(执行上下文 上下文包.上下文, 范围 工具包.范围, 原始 JSON包.原始JSON消息) (_ 工具包.工具调用, 返回错误 error) {
	if 运行时包.目标操作系统 != "darwin" && 运行时包.目标操作系统 != "linux" && 运行时包.目标操作系统 != "windows" {
		return nil, 工具包.不可用错误
	}
	参数, 文件集, 错误 := 解码补丁(原始)
	if 错误 != nil {
		return nil, 错误
	}
	当前对象 := &补丁调用{所属对象: 当前补丁工具, 范围: 范围}
	defer func() {
		if 返回错误 != nil {
			返回错误 = 错误判断包.合并(返回错误, 当前对象.关闭())
		}
	}()
	if 错误 := 当前对象.准备模块(执行上下文, 参数, 文件集); 错误 != nil {
		return nil, 错误
	}
	return 当前对象, nil
}

func (当前补丁工具 *补丁工具) 获取(执行上下文 上下文包.上下文, 当前补丁调用 *补丁调用) (func(), error) {
	for {
		if 错误 := 执行上下文.获取错误(); 错误 != nil {
			return nil, 错误
		}
		当前补丁工具.互斥锁.加锁()
		繁忙 := 切片包.包含函数(当前补丁工具.活动, 当前补丁调用.重叠)
		if !繁忙 {
			当前补丁工具.活动 = append(当前补丁工具.活动, 当前补丁调用)
			当前补丁工具.互斥锁.解锁()
			return func() {
				当前补丁工具.互斥锁.加锁()
				defer 当前补丁工具.互斥锁.解锁()
				for 索引, 活动 := range 当前补丁工具.活动 {
					if 活动 == 当前补丁调用 {
						当前补丁工具.活动 = append(当前补丁工具.活动[:索引], 当前补丁工具.活动[索引+1:]...)
						break
					}
				}
				close(当前补丁工具.变化通知)
				当前补丁工具.变化通知 = make(chan struct{})
			}, nil
		}
		变化通知 := 当前补丁工具.变化通知
		当前补丁工具.互斥锁.解锁()
		select {
		case <-执行上下文.完成():
			return nil, 执行上下文.获取错误()
		case <-变化通知:
		}
	}
}
```
