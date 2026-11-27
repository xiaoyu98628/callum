```go
package 运行

import 测试包 "testing"

func TestWindows作业清理(测试器 *测试包.测试器) { 测试进程树(测试器) }

func TestWindows拒绝设备驱动器相对并且备用流集(测试器 *测试包.测试器) {
	for _, 路径 := range []string{`C:tool.exe`, `\\?\C:\tool.exe`, `\\.\pipe\tool`, `C:\tool.exe:other`} {
		if 错误 := 校验可执行文件路径(路径); 错误 == nil {
			测试器.格式化报告致命错误("接受设备、驱动器相对或备用流路径: %s", 路径)
		}
	}
	if 错误 := 校验可执行文件路径(`C:\Program Files\tool.exe`); 错误 != nil {
		测试器.报告致命错误(错误)
	}
}
```
