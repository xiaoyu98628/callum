```go
package 运行

import (
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"
	操作系统包 "os"
	字符串包 "strings"
	系统调用包 "syscall"
	测试包 "testing"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

func Test清理诊断保留错误判断无敏感详情(测试器 *测试包.测试器) {
	for _, 测试用例 := range []struct {
		名称 string
		错误 error
		类别 string
		代码 uint64
	}{
		{"system", &操作系统包.路径错误{操作: "remove", 路径: "private-secret-path", 错误: 系统调用包.操作不允许错误}, "system", uint64(系统调用包.操作不允许错误)},
		{"joined", 错误判断包.合并(错误判断包.新建("private-secret-message"), 系统调用包.访问被拒绝错误), "system", uint64(系统调用包.访问被拒绝错误)},
		{"timeout", 上下文包.超出截止时间错误, "timeout", 0},
		{"cancelled", 上下文包.已取消错误, "cancelled", 0},
		{"closed", 操作系统包.已关闭错误, "closed", 0},
		{"unavailable", 工具包.不可用错误, "unavailable", 0},
		{"unknown", 错误判断包.新建("private-secret-error"), "unknown", 0},
	} {
		测试器.执行(测试用例.名称, func(测试器 *测试包.测试器) {
			报告 := 进程报告{清理完成: true}
			if 错误 := 报告.清理错误("execution_cleanup", 测试用例.错误); 错误 != 测试用例.错误 {
				测试器.报告致命错误("原始错误链被替换")
			}
			if 报告.清理完成 || len(报告.清理错误判断) != 1 {
				测试器.格式化报告致命错误("清理失败记录不符: %+v", 报告)
			}
			失败 := 报告.清理错误判断[0]
			if 失败.阶段 != "execution_cleanup" || 失败.类别 != 测试用例.类别 || 失败.系统代码 != 测试用例.代码 {
				测试器.格式化报告致命错误("清理诊断不符: %+v", 失败)
			}
			原始, 错误 := JSON包.序列化(报告)
			if 错误 != nil || 字符串包.包含(string(原始), "private-secret") {
				测试器.格式化报告致命错误("诊断泄漏敏感详情: %s %v", 原始, 错误)
			}
		})
	}
	清理 := 进程报告{清理完成: true}
	if 错误 := 清理.清理错误("execution_cleanup", nil); 错误 != nil || !清理.清理完成 {
		测试器.报告致命错误("成功收尾被改为失败")
	}
	原始, 错误 := JSON包.序列化(清理)
	if 错误 != nil || 字符串包.包含(string(原始), "cleanup_errors") {
		测试器.格式化报告致命错误("成功结果应省略失败诊断: %s %v", 原始, 错误)
	}
}
```
