```go
package 补丁

import (
	错误判断包 "errors"
	测试包 "testing"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

func Test补丁文本匹配并且行结尾集(测试器 *测试包.测试器) {
	for _, 测试 := range []struct {
		名称, 之前, 正文, 预期 string
		冲突             bool
	}{
		{"unicode", "标题\n旧内容\n结尾\n", "@@\n 标题\n-旧内容\n+新内容\n 结尾", "标题\n新内容\n结尾\n", false},
		{"crlf", "header\r\nold\r\ntail\r\n", "@@\n header\n-old\n+new\n tail", "header\r\nnew\r\ntail\r\n", false},
		{"no_final_newline", "header\nold", "@@\n header\n-old\n+new\n*** End of File", "header\nnew", false},
		{"mixed", "header\r\nold\nlast\r\n", "@@\n-old\n+new", "header\r\nnew\nlast\r\n", false},
		{"append", "a\n", "@@\n a\n+b\n*** End of File", "a\nb\n", false},
		{"empty", "", "@@\n+new", "new\n", false},
		{"clear", "a\n", "@@\n-a", "", false},
		{"delete_last_unterminated_line", "a\nb", "@@\n-b\n*** End of File", "a\n", false},
		{"ambiguous", "a\na\n", "@@\n-a\n+b", "", true},
		{"missing", "a\n", "@@\n-other\n+b", "", true},
		{"exact_whitespace", " a\n", "@@\n-a\n+b", "", true},
		{"anchor", "section\na\nnext\na\n", "@@ next\n-a\n+b", "section\na\nnext\nb\n", false},
		{"eof", "a\na\n", "@@\n-a\n+b\n*** End of File", "a\nb\n", false},
		{"multiple_hunks", "a\nb\nc\nd\n", "@@\n-a\n+A\n@@\n-d\n+D", "A\nb\nc\nD\n", false},
	} {
		测试器.执行(测试.名称, func(测试器 *测试包.测试器) {
			文件集, 错误 := 解析补丁("*** Begin Patch\n*** Update File: note.txt\n" + 测试.正文 + "\n*** End Patch")
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			输出, 错误 := 应用文本([]byte(测试.之前), 文件集[0])
			if 测试.冲突 {
				if !错误判断包.是否属于(错误, 工具包.冲突错误) {
					测试器.格式化报告致命错误("冲突被接受: %q %v", 输出, 错误)
				}
				return
			}
			if 错误 != nil || string(输出) != 测试.预期 {
				测试器.格式化报告致命错误("内容或换行改变: %q want %q err=%v", 输出, 测试.预期, 错误)
			}
		})
	}
}
func Test补丁拒绝不支持或者不完整语法(测试器 *测试包.测试器) {
	for _, 正文 := range []string{
		"", "*** Delete File: a", "*** Update File: a\n*** Move to: b\n@@\n-a\n+b",
		"*** Update File: a\n@@\n context", "*** Update File: a\n@@\n", "*** Add File: a\nnot-prefixed",
		"*** Add File: ../escape\n+x", "*** Add File: a\n+x\n*** Add File: a\n+y", "*** Add File: a\n+\x00",
	} {
		if _, 错误 := 解析补丁("*** Begin Patch\n" + 正文 + "\n*** End Patch"); 错误 == nil {
			测试器.格式化报告致命错误("接受了无效补丁: %q", 正文)
		}
	}
}
```
