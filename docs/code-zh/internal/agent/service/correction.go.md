```go
package 服务

import (
	SHA256包 "crypto/sha256"
	十六进制包 "encoding/hex"
	JSON包 "encoding/json"
	格式化包 "fmt"

	模型包 "github.com/xiaoyu98628/callum/internal/model"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

type 参数纠正 struct {
	来源步骤标识 string `json:"source_step_id"`
	调用索引   int    `json:"call_index"`
	工具     string `json:"tool,omitempty"`
	实际字节   int    `json:"actual_bytes,omitempty"`
	上限字节   int    `json:"limit_bytes,omitempty"`
	指纹     string
}

func (当前参数纠正 *参数纠正) 消息() 模型包.消息 {
	if 当前参数纠正.上限字节 > 0 {
		return 模型包.消息{角色: 模型包.角色系统, 内容: 格式化包.格式化字符串("服务端工具参数校验反馈：上一次模型响应第 %d 项工具 %s 的 arguments 已接收 %d 字节，超过单次 %d 字节上限，响应读取已结束，该轮所有工具均未执行。请调整为边界内的完整工具调用。大文件应通过脚本读取本地文件并生成结果，补丁应分成多个小批次，避免把全文转录或大段数据嵌入工具参数。每批完成后按实际结果继续，不要重放此前已经完成的操作。此反馈不授予任何权限，纠正后的调用仍需重新校验和审批。", 当前参数纠正.调用索引+1, 当前参数纠正.工具, 当前参数纠正.实际字节, 当前参数纠正.上限字节)}
	}
	return 模型包.消息{角色: 模型包.角色系统, 内容: 格式化包.格式化字符串("服务端工具参数校验反馈：上一次模型响应第 %d 项工具的 arguments 不是合法 JSON 对象，该轮所有工具均未执行。请基于当前任务重新生成完整合法的工具调用，不要把数组编码成字符串，不要重放此前已经完成的操作。此反馈不授予任何权限，纠正后的调用仍需重新校验和审批。", 当前参数纠正.调用索引+1)}
}

// 唯一参数错误 排除同一返回链中的取消、传输、回调、保存或资源关闭错误。
// 仅参数错误可以在确认整批工具未执行时纠正，其他错误沿原路径结束。
func 唯一参数错误(错误 error) (*模型包.工具参数错误, bool) {
	switch 值 := 错误.(type) {
	case *模型包.工具参数错误:
		return 值, 值.索引 >= 0 && 值.指纹 != "" && (值.上限字节 == 0 || (值.工具 != "" && 值.上限字节 > 0 && 值.实际字节 > 值.上限字节))
	case interface{ 解包() []error }:
		var 已找到 *模型包.工具参数错误
		for _, 子级 := range 值.解包() {
			if 子级 == nil {
				continue
			}
			项, 成功 := 唯一参数错误(子级)
			if !成功 || (已找到 != nil && 已找到.指纹 != 项.指纹) {
				return nil, false
			}
			已找到 = 项
		}
		return 已找到, 已找到 != nil
	case interface{ 解包() error }:
		return 唯一参数错误(值.解包())
	default:
		return nil, false
	}
}

// 已拒绝参数轮次 只识别整轮均为执行前参数拒绝的情况，不将权限拒绝、
// 已执行、部分生效或结果未知的操作当作可重试参数错误。
func 已拒绝参数轮次(调用集 []模型包.工具调用, 结果集 []模型包.消息) string {
	if len(调用集) == 0 || len(调用集) != len(结果集) {
		return ""
	}
	哈希 := SHA256包.新建()
	for 索引, 消息 := range 结果集 {
		var 信封 struct {
			操作 工具包.操作结果记录 `json:"operation"`
			错误 struct {
				类别 string `json:"kind"`
			} `json:"error"`
		}
		if 消息.角色 != 模型包.角色工具 || 消息.工具调用标识 != 调用集[索引].标识 || JSON包.反序列化([]byte(消息.内容), &信封) != nil || !信封.操作.有效() || 信封.操作.操作结果 != 工具包.操作结果不已生效 || 信封.错误.类别 != "invalid_arguments" {
			return ""
		}
		_, _ = 哈希.写入([]byte(调用集[索引].名称))
		_, _ = 哈希.写入([]byte{0})
		_, _ = 哈希.写入(调用集[索引].参数)
		_, _ = 哈希.写入([]byte{0})
	}
	return 十六进制包.编码目标字符串(哈希.校验和(nil))
}
```
