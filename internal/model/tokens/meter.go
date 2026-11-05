package tokens

import (
	"crypto/sha256"
	"encoding/json"

	"github.com/xiaoyu98628/callum/internal/model"
)

// EstimateResult 明确区分本地估算和同一请求前缀的远端实际输入参考。
type EstimateResult struct {
	Tokens               int64  `json:"input_tokens"`
	Method               string `json:"method"`
	ReferenceInputTokens int64  `json:"reference_input_tokens,omitempty"`
}

// Meter 只属于一个执行。只有消息前缀及工具定义完全相同，才复用上轮实际输入用量；
// 压缩改变前缀后自动失效，不累加历史计费用量，也不以较低的 usage 降低本地估算。
type Meter struct {
	prefix [][32]byte
	tools  [32]byte
	actual int64
}

func fingerprints(messages []model.Message) ([][32]byte, bool) {
	result := make([][32]byte, len(messages))
	for i, message := range messages {
		value, err := json.Marshal(message)
		if err != nil {
			return nil, false
		}
		result[i] = sha256.Sum256(value)
	}
	return result, true
}

func toolFingerprint(tools []model.ToolDefinition) ([32]byte, bool) {
	value, err := json.Marshal(tools)
	return sha256.Sum256(value), err == nil
}

func (m *Meter) Estimate(messages []model.Message, tools []model.ToolDefinition, imageTokens int64) EstimateResult {
	result := EstimateResult{Tokens: Estimate(messages, tools, imageTokens), Method: Method}
	toolHash, ok := toolFingerprint(tools)
	if m.actual <= 0 || len(messages) < len(m.prefix) || !ok || toolHash != m.tools {
		return result
	}
	prefix, ok := fingerprints(messages[:len(m.prefix)])
	if !ok {
		return result
	}
	for i, hash := range prefix {
		if hash != m.prefix[i] {
			return result
		}
	}
	delta := Estimate(messages[len(m.prefix):], nil, imageTokens) - 16
	result.Tokens = max(result.Tokens, m.actual+delta)
	result.Method, result.ReferenceInputTokens = Method+"+actual-prefix", m.actual
	return result
}

func (m *Meter) Observe(messages []model.Message, tools []model.ToolDefinition, inputTokens *int64) {
	m.actual, m.prefix = 0, nil
	if inputTokens == nil || *inputTokens <= 0 || *inputTokens > 1_000_000_000_000 {
		return
	}
	// 文件路径相同不代表图片内容相同，不能据此认定请求前缀相同。
	for _, message := range messages {
		for _, b := range message.Blocks {
			if b.Type == "image" {
				return
			}
		}
	}
	prefix, ok := fingerprints(messages)
	if !ok {
		return
	}
	toolHash, ok := toolFingerprint(tools)
	if !ok {
		return
	}
	m.actual, m.prefix, m.tools = *inputTokens, prefix, toolHash
}
