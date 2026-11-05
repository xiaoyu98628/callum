package settings

import (
	"bytes"
	"encoding/json"
	"io"
	"unicode/utf8"
)

const CompactionKey = "context_compaction"

// Compaction 以输入 Token 预算控制历史压缩；估算不改变实际 usage 或工具权限。
type Compaction struct {
	Enabled            bool  `json:"enabled"`
	TokenBudget        int64 `json:"token_budget"`
	OutputReserve      int64 `json:"output_reserve"`
	SafetyMargin       int64 `json:"safety_margin"`
	ImageTokenEstimate int64 `json:"image_token_estimate"`
	RetainTurns        int   `json:"retain_turns"`
}

// DefaultCompaction 的预留量是预算策略，不是任何模型的容量声明。
func DefaultCompaction() Compaction {
	return Compaction{Enabled: true, OutputReserve: 4096, SafetyMargin: 1024, ImageTokenEstimate: 8192, RetainTurns: 4}
}

func (c Compaction) Valid() bool {
	return c.TokenBudget >= 0 && c.TokenBudget <= 1_000_000_000 &&
		c.OutputReserve > 0 && c.OutputReserve <= 1_000_000_000 &&
		c.SafetyMargin > 0 && c.SafetyMargin <= 1_000_000_000 &&
		c.ImageTokenEstimate > 0 && c.ImageTokenEstimate <= 1_000_000_000 && c.RetainTurns >= 1 && c.RetainTurns <= 32
}

// InputBudget 从已配置窗口扣除输出及安全预留，显式输入预算只能收紧已知窗口。
// maxOutput 是声明的输出上限；只缩小规划预留，不自动设置供应商请求参数。
func (c Compaction) InputBudget(window, maxOutput int64) (int64, error) {
	if !c.Valid() || window < 0 || maxOutput < 0 {
		return 0, ErrInvalid
	}
	budget := c.TokenBudget
	if window > 0 {
		reserve := c.OutputReserve
		if maxOutput > 0 {
			reserve = min(reserve, maxOutput)
		}
		available := window - reserve - c.SafetyMargin
		if available <= 0 {
			return 0, ErrInvalid
		}
		if budget == 0 || available < budget {
			budget = available
		}
	}
	if budget <= 0 {
		return 0, ErrInvalid
	}
	return budget, nil
}

// DecodeCompaction 拒绝缺失、重复、未知字段及尾随 JSON；旧阈值只在历史快照中读取。
func DecodeCompaction(raw json.RawMessage) (Compaction, error) {
	var out Compaction
	fields, err := compactionFields(raw)
	if err != nil || len(fields) != 6 {
		return out, ErrInvalid
	}
	for key, value := range fields {
		var target any
		switch key {
		case "enabled":
			target = &out.Enabled
		case "token_budget":
			target = &out.TokenBudget
		case "output_reserve":
			target = &out.OutputReserve
		case "safety_margin":
			target = &out.SafetyMargin
		case "image_token_estimate":
			target = &out.ImageTokenEstimate
		case "retain_turns":
			target = &out.RetainTurns
		default:
			return out, ErrInvalid
		}
		if string(value) == "null" || json.Unmarshal(value, target) != nil {
			return out, ErrInvalid
		}
	}
	if !out.Valid() {
		return out, ErrInvalid
	}
	return out, nil
}

func compactionFields(raw json.RawMessage) (map[string]json.RawMessage, error) {
	if len(raw) == 0 || len(raw) > MaxValueBytes || !utf8.Valid(raw) {
		return nil, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return nil, ErrInvalid
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		token, err := d.Token()
		key, ok := token.(string)
		if err != nil || !ok || fields[key] != nil {
			return nil, ErrInvalid
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return nil, ErrInvalid
		}
		fields[key] = value
	}
	if _, err := d.Token(); err != nil {
		return nil, ErrInvalid
	}
	var trailing any
	if d.Decode(&trailing) != io.EOF {
		return nil, ErrInvalid
	}
	return fields, nil
}

func DecodeCompactionSnapshot(record Record) (CompactionSnapshot, error) {
	value, err := DecodeCompaction(record.Value)
	if err != nil || record.Key != CompactionKey || record.Revision < 1 {
		return CompactionSnapshot{}, ErrStorage
	}
	return CompactionSnapshot{Revision: record.Revision, Compaction: value}, nil
}
