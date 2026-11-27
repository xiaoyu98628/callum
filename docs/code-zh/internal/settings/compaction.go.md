```go
package 设置

import (
	字节包 "bytes"
	JSON包 "encoding/json"
	输入输出包 "io"
	UTF8包 "unicode/utf8"
)

const 压缩键 = "context_compaction"

// 压缩 以输入 词元 预算控制历史压缩；估算不改变实际 用量 或工具权限。
type 压缩 struct {
	已启用    bool  `json:"enabled"`
	词元预算   int64 `json:"token_budget"`
	输出预留   int64 `json:"output_reserve"`
	安全余量   int64 `json:"safety_margin"`
	图片词元估算 int64 `json:"image_token_estimate"`
	保留轮次集  int   `json:"retain_turns"`
}

// 默认压缩 的预留量是预算策略，不是任何模型的容量声明。
func 默认压缩() 压缩 {
	return 压缩{已启用: true, 输出预留: 4096, 安全余量: 1024, 图片词元估算: 8192, 保留轮次集: 4}
}

func (当前压缩 压缩) 有效() bool {
	return 当前压缩.词元预算 >= 0 && 当前压缩.词元预算 <= 1_000_000_000 &&
		当前压缩.输出预留 > 0 && 当前压缩.输出预留 <= 1_000_000_000 &&
		当前压缩.安全余量 > 0 && 当前压缩.安全余量 <= 1_000_000_000 &&
		当前压缩.图片词元估算 > 0 && 当前压缩.图片词元估算 <= 1_000_000_000 && 当前压缩.保留轮次集 >= 1 && 当前压缩.保留轮次集 <= 32
}

// 输入预算 从已配置窗口扣除输出及安全预留，显式输入预算只能收紧已知窗口。
// 最大输出 是声明的输出上限；只缩小规划预留，不自动设置供应商请求参数。
func (当前压缩 压缩) 输入预算(窗口, 最大输出 int64) (int64, error) {
	if !当前压缩.有效() || 窗口 < 0 || 最大输出 < 0 {
		return 0, 无效错误
	}
	预算 := 当前压缩.词元预算
	if 窗口 > 0 {
		内部预留 := 当前压缩.输出预留
		if 最大输出 > 0 {
			内部预留 = min(内部预留, 最大输出)
		}
		可用 := 窗口 - 内部预留 - 当前压缩.安全余量
		if 可用 <= 0 {
			return 0, 无效错误
		}
		if 预算 == 0 || 可用 < 预算 {
			预算 = 可用
		}
	}
	if 预算 <= 0 {
		return 0, 无效错误
	}
	return 预算, nil
}

// 解码压缩 拒绝缺失、重复、未知字段及尾随 JSON。
func 解码压缩(原始 JSON包.原始JSON消息) (压缩, error) {
	var 输出 压缩
	字段集, 错误 := 压缩字段集(原始)
	if 错误 != nil || len(字段集) != 6 {
		return 输出, 无效错误
	}
	for 键, 值 := range 字段集 {
		var 目标 any
		switch 键 {
		case "enabled":
			目标 = &输出.已启用
		case "token_budget":
			目标 = &输出.词元预算
		case "output_reserve":
			目标 = &输出.输出预留
		case "safety_margin":
			目标 = &输出.安全余量
		case "image_token_estimate":
			目标 = &输出.图片词元估算
		case "retain_turns":
			目标 = &输出.保留轮次集
		default:
			return 输出, 无效错误
		}
		if string(值) == "null" || JSON包.反序列化(值, 目标) != nil {
			return 输出, 无效错误
		}
	}
	if !输出.有效() {
		return 输出, 无效错误
	}
	return 输出, nil
}

func 压缩字段集(原始 JSON包.原始JSON消息) (map[string]JSON包.原始JSON消息, error) {
	if len(原始) == 0 || len(原始) > 最大值字节 || !UTF8包.有效(原始) {
		return nil, 无效错误
	}
	丁 := JSON包.新建解码器(字节包.新建读取器(原始))
	if 词元, 错误 := 丁.词元(); 错误 != nil || 词元 != JSON包.分隔符('{') {
		return nil, 无效错误
	}
	字段集 := map[string]JSON包.原始JSON消息{}
	for 丁.更多() {
		词元, 错误 := 丁.词元()
		键, 成功 := 词元.(string)
		if 错误 != nil || !成功 || 字段集[键] != nil {
			return nil, 无效错误
		}
		var 值 JSON包.原始JSON消息
		if 丁.解码(&值) != nil {
			return nil, 无效错误
		}
		字段集[键] = 值
	}
	if _, 错误 := 丁.词元(); 错误 != nil {
		return nil, 无效错误
	}
	var 尾部 any
	if 丁.解码(&尾部) != 输入输出包.文件结束 {
		return nil, 无效错误
	}
	return 字段集, nil
}

func 解码压缩快照(记录 记录) (压缩快照, error) {
	值, 错误 := 解码压缩(记录.值)
	if 错误 != nil || 记录.键 != 压缩键 || 记录.修订号 < 1 {
		return 压缩快照{}, 存储错误
	}
	return 压缩快照{修订号: 记录.修订号, 压缩: 值}, nil
}
```
