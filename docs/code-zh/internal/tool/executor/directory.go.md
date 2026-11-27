```go
package 执行器

import 工具包 "github.com/xiaoyu98628/callum/internal/tool"

// 目标集 返回固定实际目标的副本；命令工作目录不属于文件目标。
func (当前已准备调用 *已准备调用) 目标集() []工具包.目标 {
	var 目标集 []工具包.目标
	if 当前已准备调用.目标 != nil {
		目标集 = append(目标集, *当前已准备调用.目标)
	}
	if 当前已准备调用.计划 != nil {
		目标集 = append(目标集, 当前已准备调用.计划.目标集...)
	}
	return 目标集
}
```
