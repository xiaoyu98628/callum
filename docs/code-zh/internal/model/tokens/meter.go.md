```go
package 词元

import (
	SHA256包 "crypto/sha256"
	JSON包 "encoding/json"

	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

// 估算结果 明确区分本地估算和同一请求前缀的远端实际输入参考。
type 估算结果 struct {
	词元     int64  `json:"input_tokens"`
	方法     string `json:"method"`
	引用输入词元 int64  `json:"reference_input_tokens,omitempty"`
}

// 计量器 只属于一个执行。只有消息前缀及工具定义完全相同，才复用上轮实际输入用量；
// 压缩改变前缀后自动失效，不累加历史计费用量，也不以较低的 用量 降低本地估算。
type 计量器 struct {
	前缀  [][32]byte
	工具集 [32]byte
	实际  int64
}

func 指纹集(消息集 []模型包.消息) ([][32]byte, bool) {
	结果 := make([][32]byte, len(消息集))
	for 索引, 消息 := range 消息集 {
		值, 错误 := JSON包.序列化(消息)
		if 错误 != nil {
			return nil, false
		}
		结果[索引] = SHA256包.校验和256(值)
	}
	return 结果, true
}

func 工具指纹(工具集 []模型包.工具定义) ([32]byte, bool) {
	值, 错误 := JSON包.序列化(工具集)
	return SHA256包.校验和256(值), 错误 == nil
}

func (当前计量器 *计量器) 估算(消息集 []模型包.消息, 工具集 []模型包.工具定义, 图片词元 int64) 估算结果 {
	结果 := 估算结果{词元: 估算(消息集, 工具集, 图片词元), 方法: 方法}
	工具哈希, 成功 := 工具指纹(工具集)
	if 当前计量器.实际 <= 0 || len(消息集) < len(当前计量器.前缀) || !成功 || 工具哈希 != 当前计量器.工具集 {
		return 结果
	}
	前缀, 成功 := 指纹集(消息集[:len(当前计量器.前缀)])
	if !成功 {
		return 结果
	}
	for 索引, 哈希 := range 前缀 {
		if 哈希 != 当前计量器.前缀[索引] {
			return 结果
		}
	}
	增量 := 估算(消息集[len(当前计量器.前缀):], nil, 图片词元) - 16
	结果.词元 = max(结果.词元, 当前计量器.实际+增量)
	结果.方法, 结果.引用输入词元 = 方法+"+actual-prefix", 当前计量器.实际
	return 结果
}

func (当前计量器 *计量器) 观测(消息集 []模型包.消息, 工具集 []模型包.工具定义, 输入词元 *int64) {
	当前计量器.实际, 当前计量器.前缀 = 0, nil
	if 输入词元 == nil || *输入词元 <= 0 || *输入词元 > 1_000_000_000_000 {
		return
	}
	// 文件路径相同不代表图片内容相同，不能据此认定请求前缀相同。
	for _, 消息 := range 消息集 {
		for _, 乙 := range 消息.内容块集 {
			if 乙.类型 == "image" {
				return
			}
		}
	}
	前缀, 成功 := 指纹集(消息集)
	if !成功 {
		return
	}
	工具哈希, 成功 := 工具指纹(工具集)
	if !成功 {
		return
	}
	当前计量器.实际, 当前计量器.前缀, 当前计量器.工具集 = *输入词元, 前缀, 工具哈希
}
```
