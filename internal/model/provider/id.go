package provider

import identifier "github.com/xiaoyu98628/callum/internal/id"

// ValidID 检查标识是否为小写、带连字符的标准 UUIDv7，避免不同文本表示指向同一标识。
func ValidID(id string) bool { return identifier.ValidV7(id) }
