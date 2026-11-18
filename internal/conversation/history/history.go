package history

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/xiaoyu98628/callum/internal/model"
)

// MaxBytes 限制组装的历史内容，不是 Token 估算或容量限制。
const MaxBytes = 8 << 20

// Text 拼接内容块中的原始文字。
func Text(blocks []model.Block) string {
	var text strings.Builder
	for _, block := range blocks {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	return text.String()
}

// Title 截取首次问题前 30 个 Unicode 码点，兼容中英文。
func Title(blocks []model.Block) string {
	runes := []rune(strings.TrimSpace(Text(blocks)))
	if len(runes) == 0 {
		return "新对话"
	}
	if len(runes) > 30 {
		runes = runes[:30]
	}
	return string(runes)
}

// Fingerprint 标识实际使用的内容，防止编辑后应用旧结果；不保留原文副本。
func Fingerprint(blocks []model.Block) string {
	value, _ := json.Marshal(blocks) // Block 仅含字符串，编码不可能失败。
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
