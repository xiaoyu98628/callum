```go
package 目录管理

import (
	JSON包 "encoding/json"
	字符串包 "strings"
	测试包 "testing"
)

func Test公开目录管理页使用已编码字节预算(测试器 *测试包.测试器) {
	行集 := []供应商查看{}
	for _, 标识 := range []string{"a", "b", "c", "d"} {
		行集 = append(行集, 供应商查看{标识: 标识, 名称: 字符串包.重复("<", 300000)})
	}
	值, 错误 := 公开页(行集, false, func(当前供应商查看 供应商查看) string { return 当前供应商查看.标识 })
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	数据, 序列化错误 := JSON包.序列化(值)
	if 序列化错误 != nil || len(数据) > 7<<20 {
		测试器.格式化报告致命错误("响应未有界: %d %v", len(数据), 序列化错误)
	}
	var 结果 struct {
		项集   []供应商查看 `json:"items"`
		还有更多 bool    `json:"has_more"`
		游标   string  `json:"next_after_id"`
	}
	if 错误 := JSON包.反序列化(数据, &结果); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if !结果.还有更多 || len(结果.项集) != 3 || 结果.游标 != "c" {
		测试器.格式化报告致命错误("序列化分片未保留继续游标: %+v", 结果)
	}
}
```
