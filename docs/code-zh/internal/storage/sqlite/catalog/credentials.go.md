```go
package 目录管理

import (
	上下文包 "context"
	错误判断包 "errors"

	凭据包 "github.com/xiaoyu98628/callum/internal/credentials"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	GORM包 "gorm.io/gorm"
)

// 迁移凭据 先在事务外取得系统主密钥，再以一个事务加密所有旧凭据。
// 迁移失败保留旧值供重新尝试；安全删除 清除 SQLite 可控制的旧页内容。
func 迁移凭据(执行上下文 上下文包.上下文, 数据库 *GORM包.数据库, 密码算法 凭据包.密码算法) (返回错误 error) {
	if 数据库 == nil || 密码算法 == nil {
		return 凭据包.无效错误
	}
	defer func() {
		if 返回错误 != nil {
			return
		}
		var 模式 string
		if 错误 := 数据库.带有上下文(执行上下文).原始("PRAGMA journal_mode").扫描(&模式).错误; 错误 != nil {
			返回错误 = 错误
			return
		}
		if 模式 == "wal" {
			var 结果 struct{ 繁忙, 日志, 已保存检查点 int }
			if 错误 := 数据库.带有上下文(执行上下文).原始("PRAGMA wal_checkpoint(TRUNCATE)").扫描(&结果).错误; 错误 != nil {
				返回错误 = 错误
				return
			}
			if 结果.繁忙 != 0 || 结果.日志 > 0 {
				返回错误 = 错误判断包.新建("凭据迁移的 WAL 清理尚未完成")
			}
		}
	}()
	// 已有密文时，缺失的系统主密钥不能被自动创建的新密钥替代。
	var 之后 string
	for {
		var 已有 []表结构包.供应商
		if 错误 := 数据库.带有上下文(执行上下文).条件("credential_format=1 AND api_key<>'' AND id>?", 之后).顺序("id").上限(64).查找(&已有).错误; 错误 != nil {
			return 错误
		}
		for _, 行 := range 已有 {
			if _, 错误 := 密码算法.打开(执行上下文, 行.标识, 行.API密钥); 错误 != nil {
				return 错误
			}
			之后 = 行.标识
		}
		if len(已有) < 64 {
			break
		}
	}
	var 数量 int64
	if 错误 := 数据库.带有上下文(执行上下文).模型(&表结构包.供应商{}).条件("credential_format=0 AND api_key<>''").数量(&数量).错误; 错误 != nil {
		return 错误
	}
	if 数量 == 0 {
		return nil
	}
	if 错误 := 密码算法.准备(执行上下文); 错误 != nil {
		return 错误
	}
	return 数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		var 之后 string
		for {
			var 行集 []表结构包.供应商
			if 错误 := 事务.条件("credential_format=0 AND api_key<>'' AND id>?", 之后).顺序("id").上限(64).查找(&行集).错误; 错误 != nil {
				return 错误
			}
			for _, 行 := range 行集 {
				if len(行.API密钥) > 凭据包.最大字节 {
					return 凭据包.无效错误
				}
				值, 错误 := 密码算法.加密封装(执行上下文, 行.标识, 行.API密钥)
				if 错误 != nil {
					return 错误
				}
				结果 := 事务.模型(&表结构包.供应商{}).条件("id=? AND credential_format=0", 行.标识).更新集(map[string]any{"api_key": 值, "credential_format": 1})
				if 结果.错误 != nil || 结果.行集受影响 != 1 {
					return 错误判断包.合并(凭据包.无效错误, 结果.错误)
				}
				之后 = 行.标识
			}
			if len(行集) < 64 {
				return nil
			}
		}
	})
}
```
