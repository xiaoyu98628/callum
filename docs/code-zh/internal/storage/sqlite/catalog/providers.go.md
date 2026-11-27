```go
package 目录管理

import (
	上下文包 "context"
	时间包 "time"

	供应商包 "github.com/xiaoyu98628/callum/internal/model/provider"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	GORM包 "gorm.io/gorm"
	子句包 "gorm.io/gorm/clause"
)

const 公开供应商列集 = "id, name, protocol, base_url, api_key <> '' AS has_api_key, created_at, updated_at"

// 创建供应商 保存业务层分配标识的厂商，未提供凭据时保存空字符串。
func (当前仓储 *仓储) 创建供应商(执行上下文 上下文包.上下文, 标识 string, 输入 供应商包.供应商输入) error {
	行 := 表结构包.供应商{标识: 标识, 名称: 输入.名称, 协议: 输入.协议, 基础URL: 输入.基础URL}
	行.凭据格式 = 1
	if 输入.API密钥 != nil {
		var 错误 error
		行.API密钥, 错误 = 当前仓储.密码算法.加密封装(执行上下文, 标识, *输入.API密钥)
		if 错误 != nil {
			return 存储错误(错误)
		}
	}
	if 错误 := 当前仓储.数据库.带有上下文(执行上下文).创建(&行).错误; 错误 != nil {
		return 存储错误(错误)
	}
	return nil
}

// 更新供应商 替换厂商连接信息；API密钥 为 nil 时保留原凭据，指向空字符串时清空。
func (当前仓储 *仓储) 更新供应商(执行上下文 上下文包.上下文, 标识 string, 输入 供应商包.供应商输入) error {
	字段集 := map[string]any{
		"name": 输入.名称, "protocol": 输入.协议, "base_url": 输入.基础URL,
		"updated_at": 时间包.当前时间().UTC().Unix毫秒(),
	}
	if 输入.API密钥 != nil {
		值, 错误 := 当前仓储.密码算法.加密封装(执行上下文, 标识, *输入.API密钥)
		if 错误 != nil {
			return 存储错误(错误)
		}
		字段集["api_key"], 字段集["credential_format"] = 值, 1
	}
	return 变化通知(当前仓储.数据库.带有上下文(执行上下文).模型(&表结构包.供应商{}).条件("id = ?", 标识).更新集(字段集))
}

// 获取供应商 查询不含明文凭据的厂商信息。
func (当前仓储 *仓储) 获取供应商(执行上下文 上下文包.上下文, 标识 string) (供应商包.供应商, error) {
	var 行 供应商摘要
	错误 := 当前仓储.数据库.带有上下文(执行上下文).模型(&表结构包.供应商{}).选择(公开供应商列集).条件("id = ?", 标识).取出(&行).错误
	if 错误 != nil {
		return 供应商包.供应商{}, 存储错误(错误)
	}
	return 行.目标供应商(), nil
}

// 列表供应商集 按本地标识排序返回厂商信息，查询结果不包含明文凭据。
func (当前仓储 *仓储) 列表供应商集(执行上下文 上下文包.上下文, 选项集 ...供应商包.列表选项集) (输出 供应商包.页[供应商包.供应商], 错误 error) {
	值, 错误 := 供应商包.规范化列表选项集(选项集...)
	if 错误 != nil {
		return 输出, 错误
	}
	输出.项集 = []供应商包.供应商{}
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		标识集, 更多, 错误 := 列表页标识集(事务.模型(&表结构包.供应商{}), 值, `length(CAST(name AS BLOB))+length(CAST(protocol AS BLOB))+length(CAST(base_url AS BLOB))+1024`)
		if 错误 != nil {
			return 错误
		}
		if len(标识集) == 0 {
			return nil
		}
		var 行集 []供应商摘要
		if 错误 := 事务.模型(&表结构包.供应商{}).选择(公开供应商列集).条件("id IN ?", 标识集).顺序("id").查找(&行集).错误; 错误 != nil {
			return 错误
		}
		for _, 行 := range 行集 {
			输出.项集 = append(输出.项集, 行.目标供应商())
		}
		输出.还有更多 = 更多
		if 更多 {
			输出.下一个之后标识 = 标识集[len(标识集)-1]
		}
		return nil
	})
	return 输出, 存储错误(错误)
}

// 删除供应商 删除厂商配置，仍被模型引用时返回 供应商.被引用错误。
func (当前仓储 *仓储) 删除供应商(执行上下文 上下文包.上下文, 标识 string) error {
	return 变化通知(当前仓储.数据库.带有上下文(执行上下文).省略(子句包.关联集).条件("id = ?", 标识).删除(&表结构包.供应商{}))
}
```
