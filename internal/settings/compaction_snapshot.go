package settings

import "encoding/json"

// CompactionSnapshot 固定受理时的 Token 压缩策略及其独立版本。
type CompactionSnapshot struct {
	Revision int64 `json:"revision"`
	Compaction
}

func (s CompactionSnapshot) Valid() bool {
	return s.Revision > 0 && s.Compaction.Valid()
}

// UnmarshalJSON 严格校验版本和完整的 Token 设置，不为缺失字段补默认值。
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
	if err != nil {
		return err
	}
	*s = CompactionSnapshot{Revision: revision, Compaction: cfg}
	return nil
}
