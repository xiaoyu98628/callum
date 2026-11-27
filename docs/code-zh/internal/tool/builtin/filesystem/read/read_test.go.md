```go
package 读取测试

import (
	SHA256包 "crypto/sha256"
	十六进制包 "encoding/hex"
	JSON包 "encoding/json"
	错误判断包 "errors"
	操作系统包 "os"
	文件路径包 "path/filepath"
	测试包 "testing"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	读取包 "github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/read"
)

func Test读取参数(测试器 *测试包.测试器) {
	实例 := 读取包.新建(工具包.受保护数据{})
	for _, 测试 := range []struct{ 输入, 预期 string }{
		{`{"path":"note"}`, `{"path":"note","start_line":1,"end_line":200}`},
		{`{"path":"a/../note","start_line":5}`, `{"path":"note","start_line":5,"end_line":204}`},
		{`{"path":"note","start_line":0,"end_line":0}`, `{"path":"note","start_line":1,"end_line":200}`},
		{`{"path":"note","start_line":1,"end_line":1000}`, `{"path":"note","start_line":1,"end_line":1000}`},
	} {
		实际值, 错误 := 实例.校验(JSON包.原始JSON消息(测试.输入))
		if 错误 != nil || string(实际值) != 测试.预期 {
			测试器.格式化报告致命错误("读取参数规范化改变: %s => %s, %v", 测试.输入, 实际值, 错误)
		}
	}
	for _, 原始 := range []string{
		`null`, `{"path":"note","limit":1}`, `{"path":"note","query":"text"}`,
		`{"path":"note","start_line":null}`, `{"path":"note","end_line":-1}`,
		`{"path":"note","start_line":5,"end_line":4}`, `{"path":"note","end_line":1001}`,
		`{"path":"note","start_line":1000001}`, `{"path":"note","start_line":1.5}`,
		`{"path":"note","path":"other"}`,
		`{"path":"note","start_line":1,"start_line":2}`,
		`{"path":"note","end_line":1,"end_line":2}`,
		`{"path":"note","End_line":2}`,
		`{"path":"note"} {}`,
		`{"path":"note","start_line":"1"}`,
	} {
		if _, 错误 := 实例.校验(JSON包.原始JSON消息(原始)); !错误判断包.是否属于(错误, 工具包.无效错误) {
			测试器.格式化报告致命错误("接受读取工具的无效参数: %s, %v", 原始, 错误)
		}
		if 工具调用, 错误 := 实例.准备(测试器.上下文(), 工具包.范围{}, JSON包.原始JSON消息(原始)); !错误判断包.是否属于(错误, 工具包.无效错误) {
			if 工具调用 != nil {
				if 关闭错误 := 工具调用.关闭(); 关闭错误 != nil {
					测试器.错误文本(关闭错误)
				}
			}
			测试器.格式化报告致命错误("准备阶段接受无效参数: %s, %v", 原始, 错误)
		}
	}
}

func Test读取范围保留整体文件版本(测试器 *测试包.测试器) {
	目录 := 测试器.创建临时目录()
	数据 := []byte("first\n第二行\nlast")
	if 错误 := 操作系统包.写入文件(文件路径包.合并(目录, "note"), 数据, 0600); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	实例 := 读取包.新建(工具包.受保护数据{})
	参数, 错误 := 实例.校验(JSON包.原始JSON消息(`{"path":"note","start_line":2,"end_line":2}`))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	工具调用, 错误 := 实例.准备(测试器.上下文(), 工具包.范围{标识: "w", 根路径: 目录}, 参数)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	测试器.注册清理函数(func() {
		if 错误 := 工具调用.关闭(); 错误 != nil {
			测试器.错误文本(错误)
		}
	})
	结果, 错误 := 工具调用.执行(测试器.上下文())
	if 错误 != nil || 结果.操作结果 != 工具包.操作结果无 {
		测试器.格式化报告致命错误("读取失败: %s, %v", 结果.输出, 错误)
	}
	var 输出 struct {
		文本, SHA256 string
		启动         int `json:"start_line"`
		结束         int `json:"end_line"`
		已截取        bool
	}
	if 错误 := JSON包.反序列化(结果.输出, &输出); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	校验和 := SHA256包.校验和256(数据)
	if 输出.文本 != "第二行" || 输出.启动 != 2 || 输出.结束 != 2 || !输出.已截取 || 输出.SHA256 != 十六进制包.编码目标字符串(校验和[:]) {
		测试器.格式化报告致命错误("行范围或完整文件版本改变: %s", 结果.输出)
	}
}
```
