```go
package 设置

import (
	字节包 "bytes"
	JSON包 "encoding/json"
	错误判断包 "errors"
	输入输出包 "io"
)

const 常规键 = "general"

type 后续消息模式 string

const (
	后续消息队列 后续消息模式 = "queue"
	后续消息引导 后续消息模式 = "steer"
)

// 常规 保存客户端的常规偏好。
type 常规 struct {
	后续消息模式 后续消息模式 `json:"follow_up_mode"`
}

func 默认常规() 常规 {
	return 常规{后续消息模式: 后续消息队列}
}

// 解码常规 验证完整设置对象及唯一的字段，拒绝重复键和尾随内容。
func 解码常规(原始 JSON包.原始JSON消息) (常规, error) {
	var 值 常规
	if len(原始) == 0 || len(原始) > 最大值字节 {
		return 值, 无效错误
	}
	解码器 := JSON包.新建解码器(字节包.新建读取器(原始))
	if 启动, 错误 := 解码器.词元(); 错误 != nil || 启动 != JSON包.分隔符('{') {
		return 值, 无效错误
	}
	if 键, 错误 := 解码器.词元(); 错误 != nil || 键 != "follow_up_mode" {
		return 值, 无效错误
	}
	if 错误 := 解码器.解码(&值.后续消息模式); 错误 != nil ||
		(值.后续消息模式 != 后续消息队列 && 值.后续消息模式 != 后续消息引导) {
		return 值, 无效错误
	}
	if 结束, 错误 := 解码器.词元(); 错误 != nil || 结束 != JSON包.分隔符('}') {
		return 值, 无效错误
	}
	if _, 错误 := 解码器.词元(); 错误 != 输入输出包.文件结束 {
		return 值, 无效错误
	}
	return 值, nil
}

// 解码常规记录 区分客户端输入无效与已保存的设置损坏。
func 解码常规记录(记录 记录) (常规, error) {
	值, 错误 := 解码常规(记录.值)
	if 错误 != nil || 记录.键 != 常规键 || 记录.修订号 < 1 {
		return 常规{}, 错误判断包.合并(存储错误, 无效错误)
	}
	return 值, nil
}
```
