```go
package 历史

import (
	SHA256包 "crypto/sha256"
	十六进制包 "encoding/hex"
	JSON包 "encoding/json"
	字符串包 "strings"

	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

// 最大字节 限制组装的历史内容，不是 词元 估算或容量限制。
const 最大字节 = 8 << 20

// 文本 拼接内容块中的原始文字。
func 文本(内容块集 []模型包.内容块) string {
	var 文本 字符串包.构建器
	for _, 内容块 := range 内容块集 {
		if 内容块.类型 == "text" {
			文本.写入字符串(内容块.文本)
		}
	}
	return 文本.字符串()
}

// 标题 截取首次问题前 30 个 Unicode 码点，兼容中英文。
func 标题(内容块集 []模型包.内容块) string {
	字符集 := []rune(字符串包.裁剪空间(文本(内容块集)))
	if len(字符集) == 0 {
		return "新对话"
	}
	if len(字符集) > 30 {
		字符集 = 字符集[:30]
	}
	return string(字符集)
}

// 指纹 标识实际使用的内容，防止编辑后应用旧结果；不保留原文副本。
func 指纹(内容块集 []模型包.内容块) string {
	值, _ := JSON包.序列化(内容块集) // 内容块 仅含字符串，编码不可能失败。
	校验和 := SHA256包.校验和256(值)
	return 十六进制包.编码目标字符串(校验和[:])
}
```
