```go
package 配置模块

import (
	文件路径包 "path/filepath"
	时间包 "time"

	路径集包 "github.com/xiaoyu98628/callum/internal/config/paths"
)

// 配置 保存应用的数据路径和模型执行限制，不创建目录或打开文件。
type 配置 struct {
	数据目录    string
	数据库路径   string
	运行时目录   string
	最大并发执行集 int
	调用超时    时间包.时长
	工具超时    时间包.时长
}

// 加载 优先使用显式目录；未指定时根据构建标签选择默认目录。
// 开发构建从工作目录向上定位 Callum 项目；释放 构建使用用户家目录。
// 路径解析失败时返回错误，不切换到其他模式的数据目录。
func 加载(数据目录 string) (配置, error) {
	绝对值, 错误 := 路径集包.解析(数据目录)
	if 错误 != nil {
		return 配置{}, 错误
	}
	return 配置{数据目录: 绝对值, 数据库路径: 文件路径包.合并(绝对值, "callum.db"),
		最大并发执行集: 默认最大并发执行集, 调用超时: 默认调用超时, 工具超时: 30 * 时间包.秒}, nil
}
```
