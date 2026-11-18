package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/xiaoyu98628/callum/internal/model"
)

const UserInputTool = "request_user_input"
const AwaitingUserInput = "awaiting_user_input"

// InputQuestion 是模型请求用户确认的固定问题，不包含审批或权限字段。
type InputQuestion struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
}

// InputAnswer 只提交选项下标、补充文字或跳过；选项标签从固定问题读取。
type InputAnswer struct {
	OptionIndex *int   `json:"option_index"`
	Text        string `json:"text"`
	Skipped     bool   `json:"skipped"`
}

type UserInput struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	RunID          string `json:"run_id"`
	StepID         string `json:"step_id"`
	InputQuestion
	Status    string       `json:"status"`
	Answer    *InputAnswer `json:"answer"`
	CreatedAt int64        `json:"created_at"`
	UpdatedAt int64        `json:"updated_at"`
}

type UserInputRepository interface {
	CreateUserInput(context.Context, Step) (Step, error)
	UserInputSteps(context.Context, string, bool, int, int) ([]Step, error)
	AnswerUserInput(context.Context, string, string, InputAnswer) (Step, bool, error)
	CancelUserInput(context.Context, string) (Step, error)
}

// userInputPrompt 参考上游提问指引及选项描述，适配本服务的单问题和字符串选项。
const userInputPrompt = `Ask the user for missing information, a preference, constraint, or clarification. Use the available read tools to establish facts first. Be mindful of cognitive load on the user and prefer multiple-choice questions.

Ask one clear question at a time. Provide 2-3 mutually exclusive choices when appropriate. Put the recommended option first and suffix its label with "(Recommended)" or its equivalent in the user's language. Do not include an "Other" option. Each option is a single string and may briefly explain its tradeoff. Pass an empty options array if only a free-text answer is needed.

Ask in the user's language. The user can choose an option, answer in their own words, or skip. Complete useful independent work before requesting an answer. If a reply is required, wait for an explicit answer before dependent work. Skipping is not consent. Use the service's approval flow for tool permissions.`

func UserInputDefinition() model.ToolDefinition {
	return model.ToolDefinition{Name: UserInputTool, Description: userInputPrompt, Parameters: json.RawMessage(`{"type":"object","properties":{"question":{"type":"string","minLength":1,"maxLength":4096},"options":{"type":"array","maxItems":8,"items":{"type":"string","minLength":1,"maxLength":1024}}},"required":["question","options"],"additionalProperties":false}`)}
}

// ParseInputQuestion 拒绝重复、未知及大小写不匹配字段和尾随内容。
func ParseInputQuestion(raw []byte) (InputQuestion, error) {
	var value InputQuestion
	if len(raw) > 16<<10 || !utf8.Valid(raw) {
		return value, ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return value, ErrInvalid
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return value, ErrInvalid
		}
		seen[key] = true
		switch key {
		case "question":
			err = decoder.Decode(&value.Question)
		case "options":
			err = decoder.Decode(&value.Options)
		default:
			return value, ErrInvalid
		}
		if err != nil {
			return value, ErrInvalid
		}
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return value, ErrInvalid
	}
	if decoder.Decode(new(any)) != io.EOF || !seen["question"] || !seen["options"] {
		return value, ErrInvalid
	}
	value.Question = strings.TrimSpace(value.Question)
	if value.Question == "" || len(value.Question) > 4096 || strings.ContainsRune(value.Question, 0) || value.Options == nil || len(value.Options) > 8 {
		return value, ErrInvalid
	}
	options := map[string]bool{}
	for i, option := range value.Options {
		option = strings.TrimSpace(option)
		if option == "" || len(option) > 1024 || strings.ContainsRune(option, 0) || options[option] {
			return value, ErrInvalid
		}
		options[option] = true
		value.Options[i] = option
	}
	return value, nil
}

func ValidateInputAnswer(question InputQuestion, answer InputAnswer) error {
	if !utf8.ValidString(answer.Text) || len(answer.Text) > 4096 || strings.ContainsRune(answer.Text, 0) {
		return ErrInvalid
	}
	if answer.OptionIndex != nil && (*answer.OptionIndex < 0 || *answer.OptionIndex >= len(question.Options)) {
		return ErrInvalid
	}
	if answer.Skipped {
		if answer.OptionIndex != nil || answer.Text != "" {
			return ErrInvalid
		}
	} else if answer.OptionIndex == nil && strings.TrimSpace(answer.Text) == "" {
		return ErrInvalid
	}
	return nil
}

func InputFromStep(step Step) (UserInput, error) {
	if step.Type != "user_input" {
		return UserInput{}, ErrNotFound
	}
	question, err := ParseInputQuestion(step.Input)
	if err != nil {
		return UserInput{}, err
	}
	value := UserInput{ID: step.ID, ConversationID: step.ConversationID, RunID: step.RunID, StepID: step.ID, InputQuestion: question, Status: step.Status, CreatedAt: step.CreatedAt, UpdatedAt: step.UpdatedAt}
	if step.Status == AwaitingUserInput {
		value.Status = "pending"
	}
	if step.Status == Completed {
		var output struct {
			Answer *InputAnswer `json:"answer"`
		}
		if json.Unmarshal(step.Output, &output) != nil || output.Answer == nil || ValidateInputAnswer(question, *output.Answer) != nil {
			return UserInput{}, ErrInvalid
		}
		value.Answer, value.Status = output.Answer, "answered"
		if value.Answer.Skipped {
			value.Status = "skipped"
		}
	}
	return value, nil
}

func InputAnswerOutput(step Step, answer InputAnswer) (json.RawMessage, error) {
	question, err := ParseInputQuestion(step.Input)
	if err != nil || step.ToolCallID == nil {
		return nil, ErrInvalid
	}
	if err := ValidateInputAnswer(question, answer); err != nil {
		return nil, err
	}
	var selected *string
	if answer.OptionIndex != nil {
		selected = new(question.Options[*answer.OptionIndex])
	}
	body, err := json.Marshal(map[string]any{"question": question.Question, "selected_option": selected, "text": answer.Text, "skipped": answer.Skipped})
	if err != nil {
		return nil, err
	}
	message := model.Message{Role: model.RoleTool, ToolCallID: *step.ToolCallID, Content: string(body)}
	return json.Marshal(map[string]any{"answer": answer, "context_message": message})
}
