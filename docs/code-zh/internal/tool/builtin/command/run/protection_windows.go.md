```go
package 运行

import (
	错误判断包 "errors"
	操作系统包 "os"

	Windows包 "golang.org/x/sys/windows"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

func 检查SQLite链接集(保护 工具包.受保护数据) (返回错误 error) {
	if 保护.数据库 == "" {
		return nil
	}
	for _, 后缀 := range []string{"", "-journal", "-wal", "-shm", ".lock"} {
		文件, 错误 := 打开目标路径(保护.数据库 + 后缀)
		if 后缀 != "" && 错误判断包.是否属于(错误, 操作系统包.不存在错误) {
			continue
		}
		if 错误 != nil {
			return 错误
		}
		var 信息 Windows包.依据句柄文件信息
		错误 = Windows包.获取文件信息依据句柄(Windows包.句柄(文件.文件描述符()), &信息)
		关闭错误 := 文件.关闭()
		if 错误 != nil || 关闭错误 != nil {
			return 错误判断包.合并(错误, 关闭错误)
		}
		if 信息.数量所属链接集 != 1 {
			return 工具包.已拒绝错误
		}
	}
	return nil
}
```
