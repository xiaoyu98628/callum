```go
package 设置

import JSON包 "encoding/json"

// 压缩快照 固定受理时的 词元 压缩策略及其独立版本。
type 压缩快照 struct {
	修订号 int64 `json:"revision"`
	压缩
}

func (当前压缩快照 压缩快照) 有效() bool {
	return 当前压缩快照.修订号 > 0 && 当前压缩快照.压缩.有效()
}

// 反序列化JSON 严格校验版本和完整的 词元 设置，不为缺失字段补默认值。
func (当前压缩快照 *压缩快照) 反序列化JSON(原始 []byte) error {
	字段集, 错误 := 压缩字段集(原始)
	if 错误 != nil {
		return 错误
	}
	var 修订号 int64
	if JSON包.反序列化(字段集["revision"], &修订号) != nil || 修订号 < 1 {
		return 无效错误
	}
	delete(字段集, "revision")
	值, 错误 := JSON包.序列化(字段集)
	if 错误 != nil {
		return 错误
	}
	启动配置, 错误 := 解码压缩(值)
	if 错误 != nil {
		return 错误
	}
	*当前压缩快照 = 压缩快照{修订号: 修订号, 压缩: 启动配置}
	return nil
}
```
