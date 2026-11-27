```go
// 包 标识 统一持久化标识的 UUIDv7 文本校验，不决定业务错误或标识归属。
package 标识

import UUID包 "github.com/google/uuid"

// 有效版本7 只接受小写、带连字符且使用 RFC4122 变体 的标准 UUIDv7。
func 有效版本7(值 string) bool {
	地址, 错误 := UUID包.解析(值)
	return 错误 == nil && 地址.版本() == 7 && 地址.变体() == UUID包.RFC4122 && 地址.字符串() == 值
}
```
