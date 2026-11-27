```go
package 目录管理

import (
	上下文包 "context"
	时间包 "time"

	GORM包 "gorm.io/gorm"
	子句包 "gorm.io/gorm/clause"

	凭据包 "github.com/xiaoyu98628/callum/internal/credentials"
	供应商包 "github.com/xiaoyu98628/callum/internal/model/provider"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

// 创建模型 保存业务层分配标识的模型，保留显式 false，关联约束由数据库检查。
func (当前仓储 *仓储) 创建模型(执行上下文 上下文包.上下文, 标识 string, 输入 供应商包.模型输入) error {
	行 := 目标记录(输入)
	行.标识 = 标识
	if 错误 := 当前仓储.数据库.带有上下文(执行上下文).省略(子句包.关联集).创建(&行).错误; 错误 != nil {
		return 存储错误(错误)
	}
	return nil
}

// 更新模型 替换完整模型配置，保留记录标识和创建时间。
func (当前仓储 *仓储) 更新模型(执行上下文 上下文包.上下文, 标识 string, 输入 供应商包.模型输入) error {
	行 := 目标记录(输入)
	行.更新时间 = 时间包.当前时间().UTC().Unix毫秒()
	// 显式选择完整配置字段，包含 false、空数组以及需要清空的 空。
	return 变化通知(当前仓储.数据库.带有上下文(执行上下文).模型(&表结构包.模型{}).条件("id = ?", 标识).
		选择("ProviderID", "DisplayName", "RemoteModelID", "Enabled", "ContextWindow",
			"MaxOutputTokens", "Capabilities", "ModelType", "ReasoningLevels", "UpdatedAt").更新集(&行))
}

// 获取模型 查询模型配置，包含禁用的记录。
func (当前仓储 *仓储) 获取模型(执行上下文 上下文包.上下文, 标识 string) (供应商包.模型, error) {
	var 行 表结构包.模型
	if 错误 := 当前仓储.数据库.带有上下文(执行上下文).条件("id = ?", 标识).取出(&行).错误; 错误 != nil {
		return 供应商包.模型{}, 存储错误(错误)
	}
	return 来源记录(行)
}

// 列表模型集 按本地标识排序返回模型；可选仅 为 true 时只返回已启用记录。
// 实际调用前还需检查协议适配器是否支持配置中启用的功能。
func (当前仓储 *仓储) 列表模型集(执行上下文 上下文包.上下文, 可选仅 bool, 选项集 ...供应商包.列表选项集) (输出 供应商包.页[供应商包.模型], 错误 error) {
	值, 错误 := 供应商包.规范化列表选项集(选项集...)
	if 错误 != nil {
		return 输出, 错误
	}
	输出.项集 = []供应商包.模型{}
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		查询 := 事务.模型(&表结构包.模型{})
		if 可选仅 {
			查询 = 查询.条件("enabled = ?", true)
		}
		// 只预读有界元数据，再加载同一事务内的记录，避免先把大目录全部解码。
		标识集, 更多, 错误 := 列表页标识集(查询, 值, `length(CAST(display_name AS BLOB))+length(CAST(remote_model_id AS BLOB))+length(CAST(provider_id AS BLOB))+COALESCE(length(CAST(capabilities AS BLOB)),0)+COALESCE(length(CAST(model_type AS BLOB)),0)+COALESCE(length(CAST(reasoning_levels AS BLOB)),0)+1024`)
		if 错误 != nil {
			return 错误
		}
		if len(标识集) == 0 {
			return nil
		}
		var 行集 []表结构包.模型
		if 错误 := 事务.条件("id IN ?", 标识集).顺序("id").查找(&行集).错误; 错误 != nil {
			return 错误
		}
		for _, 行 := range 行集 {
			映射, 错误 := 来源记录(行)
			if 错误 != nil {
				return 错误
			}
			输出.项集 = append(输出.项集, 映射)
		}
		输出.还有更多 = 更多
		if 更多 {
			输出.下一个之后标识 = 标识集[len(标识集)-1]
		}
		return nil
	})
	return 输出, 存储错误(错误)
}

func 列表页标识集(查询 *GORM包.数据库, 选项集 供应商包.列表选项集, 字节表达式 string) ([]string, bool, error) {
	var 大小集 []struct {
		标识 string
		字节 int64
	}
	if 错误 := 查询.条件("id > ?", 选项集.之后标识).选择("id, " + 字节表达式 + " AS bytes").顺序("id").上限(选项集.上限 + 1).扫描(&大小集).错误; 错误 != nil {
		return nil, false, 错误
	}
	标识集 := make([]string, 0, min(选项集.上限, len(大小集)))
	var 总计 int64
	for _, 行 := range 大小集 {
		if len(标识集) == 选项集.上限 || 总计+行.字节 > 供应商包.最大列表来源字节 {
			if len(标识集) == 0 {
				return nil, false, 供应商包.无效配置错误
			}
			return 标识集, true, nil
		}
		标识集 = append(标识集, 行.标识)
		总计 += 行.字节
	}
	return 标识集, false, nil
}

// 设置模型已启用 控制模型能否用于新的运行配置解析。
func (当前仓储 *仓储) 设置模型已启用(执行上下文 上下文包.上下文, 标识 string, 已启用 bool) error {
	return 变化通知(当前仓储.数据库.带有上下文(执行上下文).模型(&表结构包.模型{}).条件("id = ?", 标识).更新集(map[string]any{
		"enabled": 已启用, "updated_at": 时间包.当前时间().UTC().Unix毫秒(),
	}))
}

// 删除模型 删除模型配置，不存在时返回 供应商.未找到错误。
func (当前仓储 *仓储) 删除模型(执行上下文 上下文包.上下文, 标识 string) error {
	return 变化通知(当前仓储.数据库.带有上下文(执行上下文).省略(子句包.关联集).条件("id = ?", 标识).删除(&表结构包.模型{}))
}

// 加载运行时 使用一次 合并 读取模型与厂商快照，不持有跨越模型调用的事务。
// 此处只读取数据，启用规则及配置校验由业务服务负责。
func (当前仓储 *仓储) 加载运行时(执行上下文 上下文包.上下文, 标识 string) (供应商包.运行时配置, error) {
	var 行 struct {
		记录       表结构包.模型 `gorm:"embedded"`
		运行时协议    string
		运行时供应商名称 string
		运行时基准URL string
		运行时API密钥 string `json:"-"`
		运行时凭据格式  int
	}
	错误 := 当前仓储.数据库.带有上下文(执行上下文).模型(&表结构包.模型{}).
		选择("models.*, p.name AS runtime_provider_name, p.protocol AS runtime_protocol, p.base_url AS runtime_base_url, p.api_key AS runtime_api_key, p.credential_format AS runtime_credential_format").
		关联("JOIN model_providers p ON p.id = models.provider_id").条件("models.id = ?", 标识).取出(&行).错误
	if 错误 != nil {
		return 供应商包.运行时配置{}, 存储错误(错误)
	}
	映射, 错误 := 来源记录(行.记录)
	if 错误 != nil {
		return 供应商包.运行时配置{}, 错误
	}
	if 行.运行时API密钥 != "" && 行.运行时凭据格式 != 1 {
		return 供应商包.运行时配置{}, 存储错误(凭据包.无效错误)
	}
	键, 错误 := 当前仓储.密码算法.打开(执行上下文, 映射.供应商标识, 行.运行时API密钥)
	if 错误 != nil {
		return 供应商包.运行时配置{}, 存储错误(错误)
	}
	return 供应商包.运行时配置{模型: 映射, 供应商名称: 行.运行时供应商名称, 协议: 行.运行时协议, 基础URL: 行.运行时基准URL, API密钥: 键}, nil
}
```
