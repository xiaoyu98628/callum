```go
package 参数测试

import (
	JSON包 "encoding/json"
	错误判断包 "errors"
	字符串包 "strings"
	测试包 "testing"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	参数包 "github.com/xiaoyu98628/callum/internal/tool/builtin/internal/arguments"
)

type 输入 struct {
	路径  string   `json:"path"`
	上限  int      `json:"limit"`
	命令  []string `json:"command"`
	版本集 []struct {
		路径     string `json:"path"`
		SHA256 string `json:"sha256"`
	} `json:"versions"`
}

func Test解码拒绝歧义并且格式错误参数(测试器 *测试包.测试器) {
	for _, 原始 := range []string{
		``, `null`, `[]`, `"object"`, `{`,
		`{"path":"first","path":"last"}`, `{"path":"first","pa\u0074h":"last"}`,
		`{"Path":"other"}`, `{"path":"a","extra":1}`, `{"path":null}`, `{"path":1}`,
		`{"limit":null}`, `{"limit":true}`, `{"limit":1.5}`, `{"limit":9223372036854775808}`,
		`{"command":["go",null]}`, `{"command":[false]}`, `{"command":{}}`,
		`{"versions":[null]}`, `{"versions":[{"path":"a","path":"b"}]}`,
		`{"versions":[{"Path":"a","sha256":"x"}]}`, `{"versions":[{"path":"a","unknown":1}]}`,
		`{"versions":[{"path":null}]}`, `{"versions":[{"path":"a","sha256":3}]}`,
		`{"path":"a"} {}`, `{"path":"a"} garbage`, string([]byte{'{', '"', 'p', 'a', 't', 'h', '"', ':', '"', 255, '"', '}'}),
	} {
		测试器.执行(原始, func(测试器 *测试包.测试器) {
			var 目标 输入
			if _, 错误 := 参数包.解码(JSON包.原始JSON消息(原始), &目标); !错误判断包.是否属于(错误, 工具包.无效错误) {
				测试器.格式化报告致命错误("接受无效参数: %q, %v", 原始, 错误)
			}
		})
	}
}

func Test解码保留带类型值集并且字段存在(测试器 *测试包.测试器) {
	var 目标 输入
	字段集, 错误 := 参数包.解码(JSON包.原始JSON消息(` {"path":"中文","command":["go","","&;>"],"limit":0,"versions":[{"path":"a","sha256":"abc"}]} `), &目标)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 目标.路径 != "中文" || len(目标.命令) != 3 || 目标.命令[1] != "" || 目标.命令[2] != "&;>" || len(目标.版本集) != 1 || 目标.版本集[0].SHA256 != "abc" || !字段集["limit"] {
		测试器.格式化报告致命错误("类型值或字段存在标记改变: %+v, %+v", 目标, 字段集)
	}
	字段集, 错误 = 参数包.解码(JSON包.原始JSON消息(`{}`), &输入{})
	if 错误 != nil || len(字段集) != 0 {
		测试器.格式化报告致命错误("缺失业务必填项应由工具判断: %+v, %v", 字段集, 错误)
	}
}

func Test解码边界输入并且嵌套(测试器 *测试包.测试器) {
	var 目标 输入
	原始 := JSON包.原始JSON消息(`{"path":"` + 字符串包.重复("x", 工具包.最大参数字节) + `"}`)
	if _, 错误 := 参数包.解码(原始, &目标); !错误判断包.是否属于(错误, 工具包.超出上限错误) {
		测试器.格式化报告致命错误("未限制输入字节: %v", 错误)
	}
	type 节点 struct {
		下一个 *节点 `json:"next"`
	}
	var 树 节点
	范围内 := 字符串包.重复(`{"next":`, 8) + `{}` + 字符串包.重复(`}`, 8)
	if _, 错误 := 参数包.解码(JSON包.原始JSON消息(范围内), &树); 错误 != nil {
		测试器.格式化报告致命错误("合法深度被拒绝: %v", 错误)
	}
	过于深度 := `{"next":` + 范围内 + `}`
	if _, 错误 := 参数包.解码(JSON包.原始JSON消息(过于深度), &树); !错误判断包.是否属于(错误, 工具包.无效错误) {
		测试器.格式化报告致命错误("未限制嵌套深度: %v", 错误)
	}
}

func Test解码字段反馈保留嵌套类型无值集(测试器 *测试包.测试器) {
	for _, 测试 := range []struct{ 原始, 字段 string }{
		{`{"command":"private-value"}`, "command"},
		{`{"versions":[{"path":"a","sha256":12345}]}`, "versions.0.sha256"},
	} {
		var 目标 输入
		_, 错误 := 参数包.解码(JSON包.原始JSON消息(测试.原始), &目标)
		详情, 成功 := 错误判断包.按类型匹配[*工具包.参数错误](错误)
		if !成功 || 详情.字段 != 测试.字段 || !字符串包.包含(详情.消息, "字符串") || 字符串包.包含(详情.错误文本(), "private-value") || 字符串包.包含(详情.错误文本(), "12345") {
			测试器.格式化报告致命错误("字段诊断不准确或回显了值: %v", 错误)
		}
	}
}
```
