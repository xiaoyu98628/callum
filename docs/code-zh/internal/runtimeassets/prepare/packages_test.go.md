```go
package 准备模块

import (
	ZIP包 "archive/zip"
	操作系统包 "os"
	文件路径包 "path/filepath"
	字符串包 "strings"
	测试包 "testing"
)

func 滚轮测试夹具(测试器 *测试包.测试器, 目录, 名称 string, 文件集 map[string]string) string {
	测试器.标记测试辅助函数()
	路径 := 文件路径包.合并(目录, 名称+".whl")
	文件, 错误 := 操作系统包.创建(路径)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	写入器 := ZIP包.新建写入器(文件)
	for 名称, 内容 := range 文件集 {
		条目, 错误 := 写入器.创建(名称)
		if 错误 != nil {
			测试器.报告致命错误(错误)
		}
		if _, 错误 := 条目.写入([]byte(内容)); 错误 != nil {
			测试器.报告致命错误(错误)
		}
	}
	if 错误 := 写入器.关闭(); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 文件.关闭(); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	return 路径
}

func Test滚轮组合保留模块集并且拒绝覆盖(测试器 *测试包.测试器) {
	基准 := 测试器.创建临时目录()
	模块 := "module/__init__.py"
	首个 := 滚轮测试夹具(测试器, 基准, "first", map[string]string{模块: "first"})
	秒 := 滚轮测试夹具(测试器, 基准, "second", map[string]string{模块: "replaced"})
	目标 := 目标{平台: "darwin", Python: 来源{版本: "3.14.10"}}
	for _, 名称 := range []string{"first", "second"} {
		目标.Python包集 = append(目标.Python包集, 包来源{名称: 名称, 版本: "1", URL: "https://files.pythonhosted.org/" + 名称 + ".whl", SHA256: 字符串包.重复("0", 64), 归档: 名称 + ".whl", 前缀: ".", 条目: 模块})
	}
	阶段 := 测试器.创建临时目录()
	if 错误 := 安装包集(测试器.上下文(), 目标, 阶段, []string{首个, 秒}); 错误 == nil {
		测试器.报告致命错误("后一个 wheel 覆盖了已安装模块")
	}
	数据, 错误 := 操作系统包.读取文件(文件路径包.合并(阶段, 包目录(目标), 文件路径包.来源斜杠(模块)))
	if 错误 != nil || string(数据) != "first" {
		测试器.格式化报告致命错误("已有模块被修改: %q %v", 数据, 错误)
	}
	if 错误 := 提取进入(测试器.上下文(), 滚轮测试夹具(测试器, 基准, "unsafe", map[string]string{"../escape": "no"}), 测试器.创建临时目录(), "."); 错误 == nil {
		测试器.报告致命错误("wheel 路径越界未拒绝")
	}
	目标.Python包集[0].URL = "https://example.org/untrusted.whl"
	if 错误 := 校验包集(目标); 错误 == nil {
		测试器.报告致命错误("未拒绝非固定归档来源")
	}
}
```
