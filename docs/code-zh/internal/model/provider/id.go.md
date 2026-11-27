```go
package 供应商

import 标识符包 "github.com/xiaoyu98628/callum/internal/id"

// 有效标识 检查标识是否为小写、带连字符的标准 UUIDv7，避免不同文本表示指向同一标识。
func 有效标识(标识 string) bool { return 标识符包.有效版本7(标识) }
```
