```go
package 迁移集

import (
	JSON包 "encoding/json"
	格式化包 "fmt"
	时间包 "time"

	UUID包 "github.com/google/uuid"
	设置包 "github.com/xiaoyu98628/callum/internal/settings"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	GORM包 "gorm.io/gorm"
)

func 设置表集() []any { return []any{&表结构包.设置{}} }

func 初始化设置(事务 *GORM包.数据库) error {
	组集 := []struct {
		键 string
		值 any
	}{
		{设置包.执行键, 设置包.默认执行()},
		{设置包.压缩键, 设置包.默认压缩()},
	}
	for _, 组 := range 组集 {
		if 错误 := 创建设置(事务, 组.键, 组.值); 错误 != nil {
			return 错误
		}
	}
	return nil
}

func 创建设置(事务 *GORM包.数据库, 键 string, 值 any) error {
	标识, 错误 := UUID包.新建UUIDv7()
	if 错误 != nil {
		return 错误
	}
	原始, 错误 := JSON包.序列化(值)
	if 错误 != nil {
		return 错误
	}
	当前时间 := 时间包.当前时间().Unix毫秒()
	记录 := 表结构包.设置{标识: 标识.字符串(), 键: 键, 值: 原始, 修订号: 1, 创建时间: 当前时间, 更新时间: 当前时间}
	return 事务.创建(&记录).错误
}

// 确保常规设置 在既有结构事务中补齐常规组，保留已保存的值和版本。
func 确保常规设置(事务 *GORM包.数据库) error {
	var 行 表结构包.设置
	错误 := 事务.条件("key=?", 设置包.常规键).上限(1).查找(&行).错误
	if 错误 != nil {
		return 错误
	}
	if 行.标识 == "" {
		return 创建设置(事务, 设置包.常规键, 设置包.默认常规())
	}
	_, 错误 = 设置包.解码常规记录(设置包.记录{键: 行.键, 值: 行.值, 修订号: 行.修订号})
	if 错误 != nil {
		return 格式化包.格式化创建错误("校验常规设置 %q: %w", 行.键, 错误)
	}
	return nil
}

func 校验设置(事务 *GORM包.数据库) error {
	if 错误 := 校验唯一索引(事务, "app_settings", "uq_settings_key", []string{"key"}); 错误 != nil {
		return 错误
	}
	var 压缩 表结构包.设置
	if 错误 := 事务.条件("key=?", 设置包.压缩键).取出(&压缩).错误; 错误 != nil {
		return 格式化包.格式化创建错误("读取上下文压缩设置: %w", 错误)
	}
	_, 错误 := 设置包.解码压缩快照(设置包.记录{键: 压缩.键, 值: 压缩.值, 修订号: 压缩.修订号})
	return 错误
}
```
