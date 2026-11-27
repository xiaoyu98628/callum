```go
package MCP

import (
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"
	时间包 "time"

	凭据包 "github.com/xiaoyu98628/callum/internal/credentials"
	业务域包 "github.com/xiaoyu98628/callum/internal/mcp"
	设置包 "github.com/xiaoyu98628/callum/internal/settings"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	GORM包 "gorm.io/gorm"
	子句包 "gorm.io/gorm/clause"
)

// 仓储 保存 MCP 配置及密文。加解密和网络请求均在写事务之外。
type 仓储 struct {
	数据库  *GORM包.数据库
	密码算法 凭据包.密码算法
}

func 新建(数据库 *GORM包.数据库, 密码算法 凭据包.密码算法) *仓储 {
	return &仓储{数据库: 数据库, 密码算法: 密码算法}
}

func (当前仓储 *仓储) 加载(执行上下文 上下文包.上下文) (设置包.MCP, int64, error) {
	var 行 表结构包.设置
	if 错误 := 当前仓储.数据库.带有上下文(执行上下文).条件("key = ?", 设置包.MCP键).取出(&行).错误; 错误 != nil {
		return 设置包.MCP{}, 0, 错误
	}
	启动配置, 错误 := 设置包.解码MCP记录(设置包.记录{键: 行.键, 值: 行.值, 修订号: 行.修订号})
	return 启动配置, 行.修订号, 错误
}

func (当前仓储 *仓储) 凭据(执行上下文 上下文包.上下文, 标识 string) (业务域包.凭据, error) {
	var 行 表结构包.MCP凭据
	错误 := 当前仓储.数据库.带有上下文(执行上下文).条件("server_id = ?", 标识).取出(&行).错误
	if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
		return 业务域包.凭据{}, nil
	}
	if 错误 != nil {
		return 业务域包.凭据{}, 错误
	}
	纯文本, 错误 := 当前仓储.密码算法.打开(执行上下文, "mcp/"+标识, 行.密文)
	if 错误 != nil {
		return 业务域包.凭据{}, 错误
	}
	var 值 业务域包.凭据
	错误 = JSON包.反序列化([]byte(纯文本), &值)
	return 值, 错误
}

func (当前仓储 *仓储) 保存(执行上下文 上下文包.上下文, 启动配置 设置包.MCP, 修订号 int64, 标识 string, 凭据 *业务域包.凭据) error {
	原始, 错误 := JSON包.序列化(启动配置)
	if 错误 != nil {
		return 错误
	}
	if _, 错误 := 设置包.解码MCP(原始); 错误 != nil {
		return 错误
	}
	已加密, 错误 := 当前仓储.加密封装(执行上下文, 标识, 凭据)
	if 错误 != nil {
		return 错误
	}
	return 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		结果 := 事务.模型(&表结构包.设置{}).条件("key = ? AND revision = ?", 设置包.MCP键, 修订号).
			更新集(map[string]any{"value": string(原始), "revision": 修订号 + 1, "updated_at": 时间包.当前时间().Unix毫秒()})
		if 结果.错误 != nil {
			return 结果.错误
		}
		if 结果.行集受影响 != 1 {
			return 设置包.冲突错误
		}
		if 凭据 == nil {
			return nil
		}
		return 写入凭据(事务, 标识, 已加密)
	})
}

func (当前仓储 *仓储) 保存凭据(执行上下文 上下文包.上下文, 标识 string, 凭据 业务域包.凭据) error {
	已加密, 错误 := 当前仓储.加密封装(执行上下文, 标识, &凭据)
	if 错误 != nil {
		return 错误
	}
	return 写入凭据(当前仓储.数据库.带有上下文(执行上下文), 标识, 已加密)
}

func (当前仓储 *仓储) 加密封装(执行上下文 上下文包.上下文, 标识 string, 值 *业务域包.凭据) (string, error) {
	if 值 == nil || 值.空() {
		return "", nil
	}
	乙, 错误 := JSON包.序列化(值)
	if 错误 != nil {
		return "", 错误
	}
	return 当前仓储.密码算法.加密封装(执行上下文, "mcp/"+标识, string(乙))
}

func 写入凭据(事务 *GORM包.数据库, 标识, 已加密 string) error {
	if 已加密 == "" {
		return 事务.条件("server_id = ?", 标识).删除(&表结构包.MCP凭据{}).错误
	}
	行 := 表结构包.MCP凭据{服务端标识: 标识, 密文: 已加密, 更新时间: 时间包.当前时间().Unix毫秒()}
	return 事务.子句集(子句包.开启冲突{列集: []子句包.列{{名称: "server_id"}}, 执行更新集: 子句包.赋值列集([]string{"ciphertext", "updated_at"})}).创建(&行).错误
}
```
