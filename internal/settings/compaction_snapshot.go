package settings

import "encoding/json"

// CompactionSnapshot 固定受理时的策略；旧快照仅用于历史审计，不自动重新执行。
type CompactionSnapshot struct {
	Revision int64 `json:"revision"`
	Compaction
	legacy json.RawMessage
}

func (s CompactionSnapshot) Valid() bool {
	return s.Revision > 0 && (s.legacy != nil || s.Compaction.Valid())
}

func (s CompactionSnapshot) MarshalJSON() ([]byte, error) {
	if s.legacy != nil {
		return s.legacy, nil
	}
	type snapshot CompactionSnapshot
	return json.Marshal(snapshot(s))
}

// UnmarshalJSON 保留旧条数/字节策略的原始快照，不能倒填当前 Token 设置。
func (s *CompactionSnapshot) UnmarshalJSON(raw []byte) error {
	fields, err := compactionFields(raw)
	if err != nil {
		return err
	}
	var revision int64
	if json.Unmarshal(fields["revision"], &revision) != nil || revision < 1 {
		return ErrInvalid
	}
	delete(fields, "revision")
	value, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	cfg, err := DecodeCompaction(value)
	if err == nil {
		*s = CompactionSnapshot{Revision: revision, Compaction: cfg}
		return nil
	}
	if len(fields) != 4 || fields["enabled"] == nil || fields["message_threshold"] == nil || fields["byte_threshold"] == nil || fields["retain_turns"] == nil {
		return ErrInvalid
	}
	var enabled bool
	var messages, size, retain int
	for key, target := range map[string]any{"enabled": &enabled, "message_threshold": &messages, "byte_threshold": &size, "retain_turns": &retain} {
		if string(fields[key]) == "null" || json.Unmarshal(fields[key], target) != nil {
			return ErrInvalid
		}
	}
	if messages < 4 || messages > 256 || size < 1024 || size > 8<<20 || retain < 1 || retain > 32 {
		return ErrInvalid
	}
	*s = CompactionSnapshot{Revision: revision, Enabled: enabled, RetainTurns: retain, legacy: append(json.RawMessage(nil), raw...)}
	return nil
}

// UpgradeCompaction 仅供数据库迁移使用。旧阈值不能换算成 Token，保留开关及保留轮次，
// 新预算由模型容量解析或用户明确配置；不改写任何历史执行快照。
func UpgradeCompaction(raw json.RawMessage) (Compaction, bool, error) {
	if value, err := DecodeCompaction(raw); err == nil {
		return value, false, nil
	}
	fields, err := compactionFields(raw)
	if err != nil {
		return Compaction{}, false, err
	}
	if len(fields) != 4 {
		return Compaction{}, false, ErrInvalid
	}
	fields["revision"] = json.RawMessage("1")
	encoded, err := json.Marshal(fields)
	if err != nil {
		return Compaction{}, false, err
	}
	var old CompactionSnapshot
	if old.UnmarshalJSON(encoded) != nil || old.legacy == nil {
		return Compaction{}, false, ErrInvalid
	}
	value := DefaultCompaction()
	value.Enabled, value.RetainTurns = old.Enabled, old.RetainTurns
	return value, true, nil
}
