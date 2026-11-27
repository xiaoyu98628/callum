```go
package 搜索测试

import (
	JSON包 "encoding/json"
	错误判断包 "errors"
	操作系统包 "os"
	文件路径包 "path/filepath"
	测试包 "testing"
	时间包 "time"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	搜索包 "github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/search"
	执行器包 "github.com/xiaoyu98628/callum/internal/tool/executor"
	注册表包 "github.com/xiaoyu98628/callum/internal/tool/registry"
)

func Test搜索参数(测试器 *测试包.测试器) {
	实例 := 搜索包.新建(工具包.受保护数据{})
	for _, 测试 := range []struct{ 输入, 预期 string }{
		{`{"path":".","query":"needle"}`, `{"path":".","limit":50,"query":"needle"}`},
		{`{"path":"a/../notes","query":"needle","limit":0}`, `{"path":"notes","limit":50,"query":"needle"}`},
		{`{"path":".","query":"needle","limit":100}`, `{"path":".","limit":100,"query":"needle"}`},
	} {
		实际值, 错误 := 实例.校验(JSON包.原始JSON消息(测试.输入))
		if 错误 != nil || string(实际值) != 测试.预期 {
			测试器.格式化报告致命错误("搜索参数规范化改变: %s => %s, %v", 测试.输入, 实际值, 错误)
		}
	}
	for _, 原始 := range []string{
		`null`, `{"path":"."}`, `{"path":".","query":""}`,
		`{"path":".","query":null}`, `{"path":".","query":"x","limit":null}`,
		`{"path":".","query":"x","limit":-1}`, `{"path":".","query":"x","limit":101}`,
		`{"path":".","query":"x","end_line":1}`, `{"path":"../outside","query":"x"}`,
		`{"path":".","path":"other","query":"x"}`,
		`{"path":".","query":"x","query":"y"}`,
		`{"path":".","query":"x","limit":1,"limit":2}`,
		`{"path":".","query":2}`,
		`{"path":".","query":"x"} {}`,
		`{"path":".","Query":"x"}`,
	} {
		if _, 错误 := 实例.校验(JSON包.原始JSON消息(原始)); !错误判断包.是否属于(错误, 工具包.无效错误) {
			测试器.格式化报告致命错误("接受搜索工具的无效参数: %s, %v", 原始, 错误)
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

func Test搜索句柄集空目录集(测试器 *测试包.测试器) {
	for _, 嵌套 := range []bool{false, true} {
		测试器.执行(map[bool]string{false: "empty_root", true: "empty_child"}[嵌套], func(测试器 *测试包.测试器) {
			目录 := 测试器.创建临时目录()
			if 嵌套 {
				if 错误 := 操作系统包.创建目录(文件路径包.合并(目录, "empty"), 0700); 错误 != nil {
					测试器.报告致命错误(错误)
				}
				if 错误 := 操作系统包.写入文件(文件路径包.合并(目录, "a.txt"), []byte("needle"), 0600); 错误 != nil {
					测试器.报告致命错误(错误)
				}
			}
			当前读取器 := 注册表包.新建()
			if 错误 := 当前读取器.注册(搜索包.新建(工具包.受保护数据{})); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			当前条目, 错误 := 执行器包.新建(当前读取器, 时间包.秒)
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			已准备, 错误 := 当前条目.准备(测试器.上下文(), 工具包.范围{标识: "workspace", 根路径: 目录}, "search_text", JSON包.原始JSON消息(`{"path":".","query":"needle"}`))
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			测试器.注册清理函数(func() {
				if 错误 := 已准备.关闭(); 错误 != nil {
					测试器.错误文本(错误)
				}
			})
			输出, 错误 := 当前条目.执行(测试器.上下文(), 已准备, 已准备.获取哈希())
			if 错误 != nil {
				测试器.格式化报告致命错误("搜索空目录失败: %v", 错误)
			}
			var 结果 struct {
				匹配集 []struct{ 路径, 文本 string } `json:"matches"`
				已截取 bool                      `json:"truncated"`
			}
			if 错误 := JSON包.反序列化(输出.输出, &结果); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			预期 := 0
			if 嵌套 {
				预期 = 1
			}
			if len(结果.匹配集) != 预期 || 结果.已截取 {
				测试器.格式化报告致命错误("空目录影响搜索结果: %s", 输出.输出)
			}
			已解析, 错误 := 文件路径包.求值符号链接集(目录)
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			if 嵌套 && (结果.匹配集[0].路径 != 文件路径包.合并(已解析, "a.txt") || 结果.匹配集[0].文本 != "needle") {
				测试器.格式化报告致命错误("其他文件的匹配结果丢失: %s", 输出.输出)
			}
		})
	}
}
```
