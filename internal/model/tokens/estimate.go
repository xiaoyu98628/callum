// Package tokens 估算统一请求的输入 Token，用于压缩规划，不提供计费用量或容量保证。
package tokens

import (
	"unicode/utf8"

	"github.com/xiaoyu98628/callum/internal/model"
)

// Method 固定估算规则版本，供步骤及摘要收益审计使用。
const Method = "unicode-v1"

// Text 对 ASCII 字母、数字、空白按三字符一 Token 估算，标点单独计数；
// 非 ASCII 按每码点二 Token、四字节码点四 Token 预留。它不是厂商 tokenizer。
func Text(value string) int64 {
	var counter TextCounter
	counter.Add(value)
	return counter.Tokens()
}

// TextCounter 按完整 UTF-8 文本分片增量计算同一段文本，不因分片边界重复取整。
type TextCounter struct{ total, run int64 }

func (c *TextCounter) Add(value string) {
	for _, r := range value {
		if r < 128 && ((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ' ' || r == '\n' || r == '\r' || r == '\t') {
			c.run++
			continue
		}
		c.total += (c.run + 2) / 3
		c.run = 0
		if r < 128 {
			c.total++
		} else if utf8.RuneLen(r) == 4 {
			c.total += 4
		} else {
			c.total += 2
		}
	}
}

func (c *TextCounter) Tokens() int64 { return c.total + (c.run+2)/3 }

// Estimate 统计有效消息内容、工具参数及定义，图片使用明确配置的单图估算，忽略路径长度。
func Estimate(messages []model.Message, tools []model.ToolDefinition, imageTokens int64) int64 {
	total := int64(16)
	for _, m := range messages {
		if m.Reasoning != nil {
			total += Text(*m.Reasoning)
		}
		total += 8 + Text(string(m.Role)) + Text(m.ToolCallID)
		if len(m.Blocks) == 0 {
			total += Text(m.Content)
		}
		for _, b := range m.Blocks {
			if b.Type == "image" {
				total += imageTokens
			} else if b.Type == "file" {
				total += Text(model.AttachmentText(b))
			} else {
				total += Text(b.Text)
			}
		}
		for _, call := range m.ToolCalls {
			total += 12 + Text(call.ID) + Text(call.Name) + Text(string(call.Arguments))
		}
	}
	for _, tool := range tools {
		total += 16 + Text(tool.Name) + Text(tool.Description) + Text(string(tool.Parameters))
	}
	return total
}
