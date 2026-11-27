```go
package 会话

import (
	比较包 "cmp"
	上下文包 "context"
	错误判断包 "errors"
	切片包 "slices"

	GORM包 "gorm.io/gorm"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	业务域包 "github.com/xiaoyu98628/callum/internal/conversation"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	智能体仓储包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/agent"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

// 历史 在同一读事务中取得执行与消息，排除重发旧分支，并预检消息总字节。
func (当前仓储 *仓储) 历史(执行上下文 上下文包.上下文, 标识, 之前 string, 上限 int) (输出 业务域包.历史页, 错误 error) {
	输出.消息集, 输出.执行集 = []业务域包.消息{}, []智能体包.执行{}
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		var 会话 表结构包.会话
		if 错误 := 事务.条件("id=?", 标识).取出(&会话).错误; 错误 != nil {
			if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
				return 业务域包.未找到错误
			}
			return 错误
		}
		消息集 := 事务.模型(&表结构包.消息{}).选择("1").条件("run_id=conversation_runs.id AND conversation_id=?", 标识)
		压缩集 := 事务.模型(&表结构包.步骤{}).选择("1").条件("run_id=conversation_runs.id AND sequence=1 AND type='context_compression' AND json_extract(input,'$.purpose')='context_compaction'")
		查询 := 事务.模型(&表结构包.执行{}).条件("conversation_id=? AND (EXISTS (?) OR EXISTS (?))", 标识, 消息集, 压缩集)
		if 之前 != "" {
			查询 = 查询.条件("id<?", 之前)
		}
		var 行集 []智能体仓储包.执行行
		if 错误 := 智能体仓储包.带有继续时间(查询).顺序("id DESC").上限(上限 + 1).查找(&行集).错误; 错误 != nil {
			return 错误
		}
		输出.还有更多 = len(行集) > 上限
		行集 = 行集[:min(len(行集), 上限)]
		var 字节 int64
		var 消息数量 int64
		for _, 行 := range 行集 {
			var 边界 struct{ 大小, 数量 int64 }
			if 错误 := 事务.模型(&表结构包.消息{}).条件("run_id=? AND conversation_id=?", 行.标识, 标识).
				选择("COALESCE(SUM(length(CAST(content AS BLOB))),0) AS size, COUNT(*) AS count").扫描(&边界).错误; 错误 != nil {
				return 错误
			}
			var 引导 struct{ 大小, 数量 int64 }
			if 错误 := 事务.模型(&表结构包.后续消息{}).
				条件("expected_run_id=? AND conversation_id=? AND status IN ?", 行.标识, 标识, []string{业务域包.后续消息引导, 业务域包.后续消息已生效}).
				选择("COALESCE(SUM(length(CAST(content AS BLOB))),0) AS size, COUNT(*) AS count").扫描(&引导).错误; 错误 != nil {
				return 错误
			}
			大小 := 边界.大小 + 引导.大小
			数量 := 边界.数量 + 引导.数量
			if 字节+大小 > 4<<20 || 消息数量+数量 > 200 {
				if len(输出.执行集) == 0 {
					return 模型包.输入上限错误
				}
				输出.还有更多 = true
				break
			}
			字节 += 大小
			消息数量 += 数量
			输出.执行集 = append(输出.执行集, 智能体仓储包.执行(行))
			输出.之前执行标识 = 行.标识
		}
		if len(输出.执行集) == 0 {
			return nil
		}
		标识集 := make([]string, len(输出.执行集))
		for 索引, 运行 := range 输出.执行集 {
			标识集[索引] = 运行.标识
		}
		var 值集 []表结构包.消息
		if 错误 := 事务.条件("conversation_id=? AND run_id IN ?", 标识, 标识集).顺序("sequence").查找(&值集).错误; 错误 != nil {
			return 错误
		}
		for _, 值 := range 值集 {
			输出.消息集 = append(输出.消息集, 消息(值))
		}
		var 引导 []引导行
		if 错误 := 引导消息集查询(事务).
			条件("f.conversation_id=? AND f.expected_run_id IN ?", 标识, 标识集).查找(&引导).错误; 错误 != nil {
			return 错误
		}
		for _, 行 := range 引导 {
			输出.消息集 = append(输出.消息集, 行.消息())
		}
		切片包.排序函数(输出.消息集, func(当前消息, 当前消息 业务域包.消息) int {
			if 顺序 := 比较包.比较(当前消息.序号, 当前消息.序号); 顺序 != 0 {
				return 顺序
			}
			if 当前消息.引导顺序 != 当前消息.引导顺序 {
				if 当前消息.引导顺序 == 0 {
					return 1
				}
				if 当前消息.引导顺序 == 0 {
					return -1
				}
				return 比较包.比较(当前消息.引导顺序, 当前消息.引导顺序)
			}
			return 比较包.比较(当前消息.标识, 当前消息.标识)
		})
		切片包.反向(输出.执行集)
		return nil
	})
	return 输出, 错误
}
```
