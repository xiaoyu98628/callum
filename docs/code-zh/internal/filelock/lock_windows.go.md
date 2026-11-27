```go
package 文件锁

import (
	错误判断包 "errors"
	操作系统包 "os"

	Windows包 "golang.org/x/sys/windows"
)

func 加锁(文件 *操作系统包.文件) error {
	var 重叠 Windows包.重叠
	错误 := Windows包.加锁文件本次执行(Windows包.句柄(文件.文件描述符()), Windows包.锁文件独占加锁|Windows包.锁文件失败立即, 0, 1, 0, &重叠)
	if 错误判断包.是否属于(错误, Windows包.错误加锁违规) {
		return 错误判断包.合并(已加锁错误, 错误)
	}
	return 错误
}
```
