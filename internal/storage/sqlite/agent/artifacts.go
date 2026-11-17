package agent

import (
	"encoding/json"

	"gorm.io/gorm"

	"github.com/xiaoyu98628/callum/internal/attachment"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	"github.com/xiaoyu98628/callum/internal/tool"
)

// recoverPublishedArtifacts 只为仍属于当前分支的助手消息恢复已确认的交付引用。
// 旧版标记为中断的消息也可补齐，原工具步骤保持不变。
func recoverPublishedArtifacts(tx *gorm.DB, statuses []string) error {
	var after string
	for {
		var messages []schema.Message
		if err := tx.Where("role = 'assistant' AND status IN ? AND id > ?", statuses, after).
			Where(`EXISTS (
				SELECT 1 FROM conversation_run_steps s
				WHERE s.run_id = conversation_messages.run_id AND s.conversation_id = conversation_messages.conversation_id
				AND s.type = 'tool_call' AND s.name = 'publish_artifact' AND s.status = 'completed'
				AND json_extract(s.definition, '$.source') = 'builtin'
				AND json_type(s.output, '$.result.published') = 'true'
				AND NOT EXISTS (SELECT 1 FROM json_each(conversation_messages.content) AS block
					WHERE json_extract(block.value, '$.attachment_ref') = json_extract(s.output, '$.result.attachment.attachment_ref'))
			)`).Order("id").Limit(16).Find(&messages).Error; err != nil {
			return err
		}
		if len(messages) == 0 {
			return nil
		}
		for _, message := range messages {
			if err := restorePublishedBlocks(tx, message); err != nil {
				return err
			}
		}
		after = messages[len(messages)-1].ID
	}
}

func restorePublishedBlocks(tx *gorm.DB, message schema.Message) error {
	seen := make(map[string]bool)
	for _, block := range message.Content {
		if block.Type == "file" {
			seen[block.AttachmentRef] = true
		}
	}
	initialCount := len(message.Content)
	var after int64
	for len(message.Content) < 64 {
		var steps []schema.Step
		if err := tx.Select("sequence", "output").
			Where("run_id = ? AND conversation_id = ? AND sequence > ?", message.RunID, message.ConversationID, after).
			Where("type = 'tool_call' AND name = 'publish_artifact' AND status = 'completed'").
			Where("json_extract(definition, '$.source') = 'builtin' AND json_type(output, '$.result.published') = 'true'").
			Order("sequence").Limit(64).Find(&steps).Error; err != nil {
			return err
		}
		if len(steps) == 0 {
			break
		}
		for _, step := range steps {
			var output struct {
				Operation tool.OutcomeRecord `json:"operation"`
				Error     json.RawMessage    `json:"error"`
				Result    struct {
					Attachment model.Block `json:"attachment"`
				} `json:"result"`
			}
			if json.Unmarshal(step.Output, &output) != nil || len(output.Error) != 0 ||
				!output.Operation.Valid() || output.Operation.Recovered || output.Operation.Outcome != tool.OutcomeNone {
				continue
			}
			block := output.Result.Attachment
			if seen[block.AttachmentRef] || !attachment.ValidPublishedBlock(block, message.ConversationID, message.RunID) {
				continue
			}
			message.Content = append(message.Content, block)
			seen[block.AttachmentRef] = true
			if len(message.Content) == 64 {
				break
			}
		}
		after = steps[len(steps)-1].Sequence
	}
	if len(message.Content) == initialCount {
		return nil
	}
	encoded, err := json.Marshal(message.Content)
	if err != nil {
		return err
	}
	// 普通恢复会统一更新活动消息的状态和时间。旧中断消息保留原结束时间。
	return tx.Model(&schema.Message{}).Where("id = ?", message.ID).Update("content", string(encoded)).Error
}
