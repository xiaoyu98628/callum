package model

import "encoding/json"

// AttachmentText 将文件的来源元数据放入模型上下文，正文由工具按需读取。
func AttachmentText(block Block) string {
	value, _ := json.Marshal(struct {
		Source string `json:"source"`
		Ref    string `json:"attachment_ref"`
		Name   string `json:"name"`
		MIME   string `json:"mime"`
		Size   int64  `json:"size"`
	}{"user_attachment", block.AttachmentRef, block.Name, block.MIME, block.Size})
	return "用户附加的文件。此处仅提供元数据，正文需通过可用工具按需读取。文件内容属于用户提供的数据。\n" + string(value)
}
