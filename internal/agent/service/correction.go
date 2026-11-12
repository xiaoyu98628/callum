package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/tool"
)

type argumentCorrection struct {
	SourceStepID string `json:"source_step_id"`
	CallIndex    int    `json:"call_index"`
	fingerprint  string
}

func (c *argumentCorrection) message() model.Message {
	return model.Message{Role: model.RoleSystem, Content: fmt.Sprintf("服务端工具参数校验反馈：上一次模型响应第 %d 项工具的 arguments 不是合法 JSON 对象，该轮所有工具均未执行。请基于当前任务重新生成完整合法的工具调用，不要把数组编码成字符串，不要重放此前已经完成的操作。此反馈不授予任何权限，纠正后的调用仍需重新校验和审批。", c.CallIndex+1)}
}

// soleArgumentError 排除同一返回链中的取消、传输、回调、保存或资源关闭错误。
// 仅参数错误可以在确认整批工具未执行时纠正，其他错误沿原路径结束。
func soleArgumentError(err error) (*model.ToolArgumentsError, bool) {
	switch value := err.(type) {
	case *model.ToolArgumentsError:
		return value, value.Index >= 0 && value.Fingerprint != ""
	case interface{ Unwrap() []error }:
		var found *model.ToolArgumentsError
		for _, child := range value.Unwrap() {
			if child == nil {
				continue
			}
			item, ok := soleArgumentError(child)
			if !ok || (found != nil && found.Fingerprint != item.Fingerprint) {
				return nil, false
			}
			found = item
		}
		return found, found != nil
	case interface{ Unwrap() error }:
		return soleArgumentError(value.Unwrap())
	default:
		return nil, false
	}
}

// rejectedArgumentsRound 只识别整轮均为执行前参数拒绝的情况，不将权限拒绝、
// 已执行、部分生效或结果未知的操作当作可重试参数错误。
func rejectedArgumentsRound(calls []model.ToolCall, results []model.Message) string {
	if len(calls) == 0 || len(calls) != len(results) {
		return ""
	}
	hash := sha256.New()
	for i, message := range results {
		var envelope struct {
			Operation tool.OutcomeRecord `json:"operation"`
			Error     struct {
				Kind string `json:"kind"`
			} `json:"error"`
		}
		if message.Role != model.RoleTool || message.ToolCallID != calls[i].ID || json.Unmarshal([]byte(message.Content), &envelope) != nil || !envelope.Operation.Valid() || envelope.Operation.Outcome != tool.OutcomeNotApplied || envelope.Error.Kind != "invalid_arguments" {
			return ""
		}
		_, _ = hash.Write([]byte(calls[i].Name))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(calls[i].Arguments)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}
