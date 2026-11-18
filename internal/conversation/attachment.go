package conversation

import "errors"

var ErrAttachmentChanged = errors.New("附件所属消息已变化，请从当前消息重新另存")

// AttachmentChunk 是按固定消息索引读取的有界原始附件片段；Data 由 JSON 编码为 Base64。
type AttachmentChunk struct {
	RunID      string `json:"run_id"`
	SHA256     string `json:"sha256"`
	Name       string `json:"name"`
	MIME       string `json:"mime"`
	Size       int64  `json:"size"`
	Offset     int64  `json:"offset"`
	Data       []byte `json:"data"`
	NextOffset *int64 `json:"next_offset"`
}
