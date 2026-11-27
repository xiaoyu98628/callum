```go
package 会话

import (
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"
	切片包 "slices"
	时间包 "time"

	GORM包 "gorm.io/gorm"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	业务域包 "github.com/xiaoyu98628/callum/internal/conversation"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

// 仓储 实现会话业务事务，借用连接，不持有跨调用事务。
type 仓储 struct{ 数据库 *GORM包.数据库 }

var _ 业务域包.仓储 = (*仓储)(nil)

func 新建仓储(数据库 *GORM包.数据库) (*仓储, error) {
	if 数据库 == nil {
		return nil, 业务域包.无效错误
	}
	return &仓储{数据库: 数据库}, nil
}

func 转换(行 表结构包.会话) 业务域包.会话 {
	return 业务域包.会话{标识: 行.标识, 工作区标识: 行.工作区标识, 标题: 行.标题, 标题来源: 行.标题来源, 模型: 行.模型, 置顶位于: 行.置顶位于, 已归档位于: 行.已归档位于, 删除时间: 行.删除时间, 创建时间: 行.创建时间, 更新时间: 行.更新时间}
}

func 消息(行 表结构包.消息) 业务域包.消息 {
	return 业务域包.消息{标识: 行.标识, 会话标识: 行.会话标识, 执行标识: 行.执行标识, 序号: 行.序号, 角色: 行.角色, 内容: 行.内容, 状态: 行.状态, 创建时间: 行.创建时间, 更新时间: 行.更新时间, 已完成位于: 行.已完成位于}
}

func (当前仓储 *仓储) 获取(执行上下文 上下文包.上下文, 标识 string) (业务域包.会话, error) {
	var 行 表结构包.会话
	错误 := 当前仓储.数据库.带有上下文(执行上下文).条件("id = ?", 标识).取出(&行).错误
	if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
		错误 = 业务域包.未找到错误
	}
	return 转换(行), 错误
}

func (当前仓储 *仓储) 列表(执行上下文 上下文包.上下文, 当前筛选 业务域包.筛选) ([]业务域包.会话, error) {
	查询 := 当前仓储.数据库.带有上下文(执行上下文).模型(&表结构包.会话{}).选择("conversations.*").关联("LEFT JOIN workspaces ON workspaces.id = conversations.workspace_id")
	if 当前筛选.工作区标识 != nil {
		查询 = 查询.条件("conversations.workspace_id = ?", *当前筛选.工作区标识)
	}
	if 当前筛选.未绑定 {
		查询 = 查询.条件("conversations.workspace_id IS NULL")
	}
	if 当前筛选.已删除 {
		查询 = 查询.条件("conversations.deleted_at IS NOT NULL")
	} else {
		查询 = 查询.条件("conversations.deleted_at IS NULL AND (workspaces.id IS NULL OR workspaces.deleted_at IS NULL)")
		if 当前筛选.已归档 {
			查询 = 查询.条件("conversations.archived_at IS NOT NULL")
		} else {
			查询 = 查询.条件("conversations.archived_at IS NULL AND (workspaces.id IS NULL OR workspaces.archived_at IS NULL)")
		}
	}
	var 行集 []表结构包.会话
	if 错误 := 查询.顺序("conversations.pinned_at DESC").顺序("conversations.updated_at DESC").顺序("conversations.id DESC").上限(当前筛选.上限).偏移(当前筛选.偏移).查找(&行集).错误; 错误 != nil {
		return nil, 错误
	}
	输出 := make([]业务域包.会话, 0, len(行集))
	for _, 行 := range 行集 {
		输出 = append(输出, 转换(行))
	}
	return 输出, nil
}

func 繁忙(事务 *GORM包.数据库, 标识 string) error {
	var 数量 int64
	if 错误 := 事务.模型(&表结构包.执行{}).条件("conversation_id = ? AND status IN ?", 标识, []string{"running", "awaiting_approval", "awaiting_user_input"}).数量(&数量).错误; 错误 != nil {
		return 错误
	}
	if 数量 != 0 {
		return 业务域包.繁忙错误
	}
	return nil
}

func 活动工作区(事务 *GORM包.数据库, 标识 *string) error {
	if 标识 == nil {
		return nil
	}
	var 行 表结构包.工作区
	if 错误 := 事务.条件("id = ?", *标识).取出(&行).错误; 错误 != nil {
		if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
			return 业务域包.未找到错误
		}
		return 错误
	}
	if 行.删除时间 != nil || 行.已归档位于 != nil {
		return 业务域包.非活动错误
	}
	return nil
}

func (当前仓储 *仓储) 更新(执行上下文 上下文包.上下文, 标识 string, 当前变更 业务域包.变更) (输出 业务域包.会话, 错误 error) {
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		var 行 表结构包.会话
		if 错误 := 事务.条件("id = ?", 标识).取出(&行).错误; 错误 != nil {
			if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
				return 业务域包.未找到错误
			}
			return 错误
		}
		if 当前变更.模型 != nil || 当前变更.工作区标识 != nil || 当前变更.解除关联工作区 || 当前变更.已归档 != nil || 当前变更.已删除 != nil {
			if 错误 := 繁忙(事务, 标识); 错误 != nil {
				return 错误
			}
		}
		当前时间 := 时间包.当前时间().Unix毫秒()
		更新集 := map[string]any{"updated_at": 当前时间}
		if 当前变更.标题 != nil {
			更新集["title"] = *当前变更.标题
			更新集["title_source"] = "manual"
		}
		if 当前变更.模型 != nil {
			行.模型 = *当前变更.模型
			if 错误 := 事务.模型(&行).选择("Model").更新集(&行).错误; 错误 != nil {
				return 错误
			}
		}
		if 当前变更.工作区标识 != nil {
			if 错误 := 活动工作区(事务, 当前变更.工作区标识); 错误 != nil {
				return 错误
			}
			更新集["workspace_id"] = *当前变更.工作区标识
		}
		if 当前变更.解除关联工作区 {
			更新集["workspace_id"] = nil
		}
		for 键, 命令行参数 := range map[string]*bool{"pinned_at": 当前变更.置顶, "archived_at": 当前变更.已归档, "deleted_at": 当前变更.已删除} {
			if 命令行参数 != nil {
				if *命令行参数 {
					更新集[键] = 当前时间
				} else {
					更新集[键] = nil
				}
			}
		}
		if 错误 := 事务.模型(&行).更新集(更新集).错误; 错误 != nil {
			return 错误
		}
		if 错误 := 事务.条件("id = ?", 标识).取出(&行).错误; 错误 != nil {
			return 错误
		}
		输出 = 转换(行)
		return nil
	})
	return 输出, 错误
}

// 消息 查询指定会话的固定消息，用于读取附图。
func (当前仓储 *仓储) 消息(执行上下文 上下文包.上下文, 会话标识, 消息标识 string) (业务域包.消息, error) {
	var 行 表结构包.消息
	错误 := 当前仓储.数据库.带有上下文(执行上下文).条件("conversation_id = ? AND id = ?", 会话标识, 消息标识).取出(&行).错误
	if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
		return 引导消息(当前仓储.数据库.带有上下文(执行上下文), 会话标识, 消息标识)
	}
	if 错误 != nil {
		return 业务域包.消息{}, 错误
	}
	return 消息(行), nil
}

func (当前仓储 *仓储) 消息集(执行上下文 上下文包.上下文, 标识 string, 之前 int64, 上限 int) ([]业务域包.消息, error) {
	if _, 错误 := 当前仓储.获取(执行上下文, 标识); 错误 != nil {
		return nil, 错误
	}
	查询 := 当前仓储.数据库.带有上下文(执行上下文).条件("conversation_id = ?", 标识)
	if 之前 > 0 {
		查询 = 查询.条件("sequence < ?", 之前)
	}
	// 先读取大小，控制整个历史查询的内存和协议响应，不把 上限 当作字节上限。
	var 大小集 []struct {
		标识 string
		字节 int64
	}
	if 错误 := 查询.模型(&表结构包.消息{}).选择("id, length(CAST(content AS BLOB)) AS bytes").顺序("sequence DESC").上限(上限).查找(&大小集).错误; 错误 != nil {
		return nil, 错误
	}
	标识集 := make([]string, 0, len(大小集))
	总计 := int64(0)
	for _, 行 := range 大小集 {
		if 总计+行.字节+1024 > 7<<20 {
			if len(标识集) == 0 {
				return nil, 智能体包.输出上限错误
			}
			break
		}
		标识集 = append(标识集, 行.标识)
		总计 += 行.字节 + 1024
	}
	if len(标识集) == 0 {
		return []业务域包.消息{}, nil
	}
	var 行集 []表结构包.消息
	if 错误 := 当前仓储.数据库.带有上下文(执行上下文).条件("id IN ?", 标识集).顺序("sequence DESC").查找(&行集).错误; 错误 != nil {
		return nil, 错误
	}
	输出 := make([]业务域包.消息, 0, len(行集))
	for _, 行 := range 切片包.向后(行集) {
		输出 = append(输出, 消息(行))
	}
	return 输出, nil
}

// 附件 只从当前消息分支查询输入或交付引用。已替换消息和其他会话不授予读取权限。
func (当前仓储 *仓储) 附件(执行上下文 上下文包.上下文, 会话标识, 引用 string) (模型包.内容块, error) {
	const 匹配文件 = "json_extract(entry.value, '$.attachment_ref') = ? AND json_extract(entry.value, '$.type') = 'file'"
	数据库 := 当前仓储.数据库.带有上下文(执行上下文)
	消息集 := 数据库.模型(&表结构包.消息{}).表("conversation_messages AS m").选择("entry.value").
		关联("JOIN json_each(m.content) AS entry").
		条件("m.conversation_id = ? AND m.role IN ('user', 'assistant')", 会话标识).
		条件(匹配文件, 引用)
	用户消息 := 数据库.模型(&表结构包.消息{}).选择("1").条件("run_id = f.expected_run_id AND role = 'user'")
	引导 := 数据库.模型(&表结构包.后续消息{}).表("conversation_followups AS f").选择("entry.value").
		关联("JOIN json_each(f.content) AS entry").
		条件("f.conversation_id = ? AND f.status IN ?", 会话标识, []string{业务域包.后续消息引导, 业务域包.后续消息已生效}).
		条件("EXISTS (?)", 用户消息).
		条件(匹配文件, 引用)

	// 两个来源共用一次查询的快照，避免读到重发前后的不同分支。
	var 行 struct{ 值 string }
	错误 := 数据库.表("(? UNION ALL ?) AS attachments", 消息集, 引导).选择("value").取出(&行).错误
	if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
		return 模型包.内容块{}, 业务域包.未找到错误
	}
	if 错误 != nil {
		return 模型包.内容块{}, 错误
	}
	var 内容块 模型包.内容块
	if 错误 := JSON包.反序列化([]byte(行.值), &内容块); 错误 != nil {
		return 内容块, 错误
	}
	return 内容块, nil
}
```
