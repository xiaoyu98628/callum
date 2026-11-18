package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

const GeneralKey = "general"

type FollowUpMode string

const (
	FollowUpQueue FollowUpMode = "queue"
	FollowUpSteer FollowUpMode = "steer"
)

// General 保存客户端的常规偏好。
type General struct {
	FollowUpMode FollowUpMode `json:"follow_up_mode"`
}

func DefaultGeneral() General {
	return General{FollowUpMode: FollowUpQueue}
}

// DecodeGeneral 验证完整设置对象及唯一的字段，拒绝重复键和尾随内容。
func DecodeGeneral(raw json.RawMessage) (General, error) {
	var value General
	if len(raw) == 0 || len(raw) > MaxValueBytes {
		return value, ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if start, err := decoder.Token(); err != nil || start != json.Delim('{') {
		return value, ErrInvalid
	}
	if key, err := decoder.Token(); err != nil || key != "follow_up_mode" {
		return value, ErrInvalid
	}
	if err := decoder.Decode(&value.FollowUpMode); err != nil ||
		(value.FollowUpMode != FollowUpQueue && value.FollowUpMode != FollowUpSteer) {
		return value, ErrInvalid
	}
	if end, err := decoder.Token(); err != nil || end != json.Delim('}') {
		return value, ErrInvalid
	}
	if _, err := decoder.Token(); err != io.EOF {
		return value, ErrInvalid
	}
	return value, nil
}

// DecodeGeneralRecord 区分客户端输入无效与已保存的设置损坏。
func DecodeGeneralRecord(record Record) (General, error) {
	value, err := DecodeGeneral(record.Value)
	if err != nil || record.Key != GeneralKey || record.Revision < 1 {
		return General{}, errors.Join(ErrStorage, ErrInvalid)
	}
	return value, nil
}
