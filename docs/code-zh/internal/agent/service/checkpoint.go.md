```go
package 服务

import (
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"
	时间包 "time"

	UUID包 "github.com/google/uuid"
	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	会话包 "github.com/xiaoyu98628/callum/internal/conversation"
	压缩包 "github.com/xiaoyu98628/callum/internal/conversation/compaction"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	词元包 "github.com/xiaoyu98628/callum/internal/model/tokens"
)

func 检查点输入(基准 *智能体包.上下文检查点, 消息集 []模型包.消息) []模型包.消息 {
	输入 := []模型包.消息{{角色: 模型包.角色系统, 内容: 压缩包.提示词}}
	if 基准 != nil {
		输入 = append(输入, 基准.消息())
	}
	输入 = append(输入, 消息集...)
	return append(输入, 模型包.消息{角色: 模型包.角色用户, 内容: 压缩包.请求})
}

// 压缩执行 每次只消费完整模型及工具批次。之前 保护正在执行的批次；保留最后 保留最近完整批次。
// 新摘要通过结构、容量、缩减和来源校验后才生效；原始步骤及实际 用量 不变。
func (当前引擎 *引擎) 压缩执行(执行上下文 上下文包.上下文, 本次执行 会话包.执行, 已解析 智能体包.已解析, 执行标识 string, 之前 int64, 保留最后 bool) (*智能体包.上下文检查点, int, error) {
	基准, 错误 := 当前引擎.仓储.最新检查点(执行上下文, 本次执行.会话.标识, 执行标识)
	if 错误 != nil {
		return nil, 0, 操作存储错误(执行上下文, 错误)
	}
	游标 := int64(0)
	if 基准 != nil {
		游标 = 基准.通过
	}
	var 来源集 []智能体包.批次来源
	var 消息集 []模型包.消息
	for {
		批次, 错误 := 当前引擎.仓储.上下文批次(执行上下文, 本次执行.会话.标识, 执行标识, 游标)
		if 错误 != nil {
			if 容量错误(错误) && len(来源集) > 0 {
				break
			}
			return nil, 0, 错误
		}
		if 批次 == nil || (之前 > 0 && 批次.通过 >= 之前) {
			break
		}
		if 保留最后 {
			下一个, 错误 := 当前引擎.仓储.上下文批次(执行上下文, 本次执行.会话.标识, 执行标识, 批次.通过)
			if 错误 != nil && !容量错误(错误) {
				return nil, 0, 错误
			}
			if 错误 == nil && 下一个 == nil {
				break
			}
		}
		if len(批次.消息集) == 0 {
			break
		}
		候选 := append(append([]模型包.消息(nil), 消息集...), 批次.消息集...)
		原始, 错误 := JSON包.序列化(候选)
		if 错误 != nil {
			return nil, 0, 错误
		}
		if len(候选) > 248 || len(原始) > 7<<20 {
			if len(来源集) == 0 {
				return nil, 0, 模型包.输入上限错误
			}
			break
		}
		if 错误 := 摘要容量(执行上下文, 本次执行, 已解析, 检查点输入(基准, 候选)); 错误 != nil {
			if !容量错误(错误) || len(来源集) == 0 {
				return nil, 0, 错误
			}
			break
		}
		消息集 = 候选
		来源集 = append(来源集, 批次.批次来源)
		游标 = 批次.通过
	}
	if len(来源集) == 0 {
		return nil, 0, nil
	}
	标识, 错误 := UUID包.新建UUIDv7()
	if 错误 != nil {
		return nil, 0, 错误
	}
	检查点 := 智能体包.上下文检查点{版本: 智能体包.上下文检查点版本, 标识: 标识.字符串(), 会话标识: 本次执行.会话.标识, 来源执行标识: 执行标识, 通过: 游标, 来源集: 来源集}
	if 基准 != nil {
		检查点.基准标识, 检查点.之后 = 基准.标识, 基准.通过
	}
	输入, 错误 := JSON包.序列化(map[string]any{"purpose": "context_compaction", "trigger": map[string]string{"type": "automatic", "reason": "context_capacity"}, "source_run_id": 执行标识, "after_step_sequence": 检查点.之后, "through_step_sequence": 游标, "source_batches": 来源集})
	if 错误 != nil {
		return nil, 0, 错误
	}
	当前时间 := 时间包.当前时间().Unix毫秒()
	步骤, 错误 := 当前引擎.仓储.创建步骤(执行上下文, 智能体包.步骤{标识: 检查点.标识, 会话标识: 本次执行.会话.标识, 执行标识: 本次执行.执行.标识, 类型: "context_compression", 名称: "压缩已完成过程", 状态: 智能体包.运行中, 输入: 输入, 创建时间: 当前时间, 开始时间: &当前时间, 更新时间: 当前时间})
	if 错误 != nil {
		return nil, 0, 操作存储错误(执行上下文, 错误)
	}
	if 错误 := 当前引擎.事件(本次执行, "step.created", 步骤.标识, "", 步骤); 错误 != nil {
		return nil, 0, 错误
	}
	值, 错误 := 当前引擎.生成检查点(执行上下文, 本次执行, 已解析, 步骤, 检查点, 基准, 消息集)
	if 错误 != nil {
		return nil, 0, 错误
	}
	return 值, len(消息集), nil
}

func (当前引擎 *引擎) 生成检查点(执行上下文 上下文包.上下文, 本次执行 会话包.执行, 已解析 智能体包.已解析, 步骤 智能体包.步骤, 值 智能体包.上下文检查点, 基准 *智能体包.上下文检查点, 消息集 []模型包.消息) (输出 *智能体包.上下文检查点, 返回错误 error) {
	已提交 := false
	defer func() {
		if 已提交 {
			return
		}
		清理, 取消函数 := 清理上下文(执行上下文)
		defer 取消函数()
		状态, 类别, 消息 := 失败(返回错误)
		当前时间 := 时间包.当前时间().Unix毫秒()
		步骤.状态, 步骤.错误类别, 步骤.错误消息, 步骤.结束时间, 步骤.更新时间 = 状态, &类别, &消息, &当前时间, 当前时间
		if 错误 := 当前引擎.仓储.保存步骤(清理, 步骤); 错误 != nil {
			返回错误 = 错误判断包.合并(返回错误, 智能体包.存储错误, 错误)
		} else {
			_ = 当前引擎.事件(本次执行, "step.updated", 步骤.标识, "", 步骤)
		}
	}()
	结果, 错误 := 当前引擎.摘要模型(执行上下文, 本次执行, 已解析, 步骤, 检查点输入(基准, 消息集), 值.标识)
	if 错误 != nil {
		return nil, 错误
	}
	if len(结果.工具调用集) != 0 {
		return nil, 模型包.无效响应错误
	}
	值.摘要, 错误 = 压缩包.校验(结果.文本)
	if 错误 != nil {
		return nil, 错误
	}
	来源 := 消息集
	if 基准 != nil {
		来源 = append([]模型包.消息{基准.消息()}, 来源...)
	}
	目标 := []模型包.消息{值.消息()}
	先前, 错误 := JSON包.序列化(来源)
	if 错误 != nil {
		return nil, 错误
	}
	之后, 错误 := JSON包.序列化(目标)
	if 错误 != nil {
		return nil, 错误
	}
	启动配置 := 本次执行.执行.设置快照.压缩
	之前词元, 之后词元 := 词元包.估算(来源, nil, 启动配置.图片词元估算), 词元包.估算(目标, nil, 启动配置.图片词元估算)
	if len(之后) >= len(先前) || 之后词元 >= 之前词元 || len(目标) > len(来源) {
		return nil, 压缩包.无减少错误
	}
	if 错误 := 摘要容量(执行上下文, 本次执行, 已解析, 目标); 错误 != nil {
		return nil, 错误
	}
	值.统计, 错误 = JSON包.序列化(map[string]any{"before_messages": len(来源), "after_messages": 1, "before_bytes": len(先前), "after_bytes": len(之后), "estimated_before_tokens": 之前词元, "estimated_after_tokens": 之后词元, "estimation_method": 词元包.方法})
	if 错误 != nil {
		return nil, 错误
	}
	if 错误 := 执行上下文.获取错误(); 错误 != nil {
		return nil, 错误
	}
	if 错误 := 当前引擎.仓储.保存检查点(执行上下文, 值); 错误 != nil {
		return nil, 操作存储错误(执行上下文, 错误)
	}
	已提交 = true
	当前时间 := 时间包.当前时间().Unix毫秒()
	步骤.状态, 步骤.结束时间, 步骤.更新时间 = 智能体包.已完成, &当前时间, 当前时间
	步骤.输出, 错误 = JSON包.序列化(map[string]any{"compressed": true, "checkpoint": 值})
	if 错误 != nil {
		return &值, 错误
	}
	return &值, 当前引擎.事件(本次执行, "step.updated", 步骤.标识, "", 步骤)
}
```
