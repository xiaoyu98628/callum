package chatcompletions

import (
	"encoding/json"
	"strings"

	"github.com/xiaoyu98628/callum/internal/model"
)

func captureUsage(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return nil, model.ErrInvalidResponse
	}
	return append(json.RawMessage(nil), raw...), nil
}

func capturedOutput(format string, response json.RawMessage, extensions ...map[string][]json.RawMessage) (json.RawMessage, error) {
	var extra map[string][]json.RawMessage
	if len(extensions) > 0 {
		extra = extensions[0]
	}
	return json.Marshal(struct {
		Protocol         string                       `json:"protocol"`
		Format           string                       `json:"format"`
		Response         json.RawMessage              `json:"response"`
		StreamExtensions map[string][]json.RawMessage `json:"stream_extensions,omitempty"`
	}{"chatcompletions", format, response, extra})
}

// streamCapture 保存组装结果而非 SSE 包；原始 JSON 字段避免 SDK 宽松解码改写 usage。
type streamCapture struct {
	root       map[string]json.RawMessage
	choice     map[string]json.RawMessage
	message    map[string]json.RawMessage
	text       map[string]*strings.Builder
	usage      json.RawMessage
	extensions map[string][]json.RawMessage
}

func (c *streamCapture) add(raw string) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return err
	}
	if c.root == nil {
		c.root = make(map[string]json.RawMessage)
		c.choice = make(map[string]json.RawMessage)
		c.message = make(map[string]json.RawMessage)
		c.text = make(map[string]*strings.Builder)
		c.extensions = make(map[string][]json.RawMessage)
	}
	for key, value := range root {
		if key != "choices" && key != "usage" {
			c.root[key] = value
		}
	}
	if value, ok := root["usage"]; ok && string(value) != "null" {
		usage, err := captureUsage(value)
		if err != nil {
			return err
		}
		c.usage = usage
	}
	var choices []map[string]json.RawMessage
	if err := json.Unmarshal(root["choices"], &choices); err != nil {
		return err
	}
	if len(choices) != 1 {
		return nil
	}
	for key, value := range choices[0] {
		if key != "delta" {
			if key == "finish_reason" && (string(value) == "null" || len(value) == 0) {
				continue
			}
			if key == "logprobs" && string(value) == "null" {
				if _, exists := c.choice[key]; exists {
					continue
				}
			}
			if key == "logprobs" && string(value) != "null" {
				merged, err := mergeLogprobs(c.choice[key], value)
				if err != nil {
					return err
				}
				c.choice[key] = merged
				continue
			}
			c.choice[key] = value
		}
	}
	var delta map[string]json.RawMessage
	if err := json.Unmarshal(choices[0]["delta"], &delta); err != nil {
		return err
	}
	for key, value := range delta {
		if key == "content" || key == "refusal" || key == "reasoning_content" {
			var text string
			if string(value) == "null" {
				continue
			}
			if err := json.Unmarshal(value, &text); err != nil {
				return model.ErrInvalidResponse
			}
			builder := c.text[key]
			if builder == nil {
				builder = &strings.Builder{}
				c.text[key] = builder
			}
			builder.WriteString(text)
		} else if key == "role" {
			c.message[key] = value
		} else {
			// 未知增量字段不猜测其合并语义，按字段保留全部片段，避免只剩最后一片。
			c.extensions[key] = append(c.extensions[key], value)
		}
	}
	return nil
}

func (c *streamCapture) result(requestID string) (model.Response, error) {
	result := model.Response{RequestID: requestID, Usage: c.usage}
	if c.root == nil {
		return result, nil
	}
	for key, builder := range c.text {
		value, err := json.Marshal(builder.String())
		if err != nil {
			return result, err
		}
		c.message[key] = value
		if key == "content" {
			result.Text = builder.String()
		}
	}
	message, err := json.Marshal(c.message)
	if err != nil {
		return result, err
	}
	c.choice["message"] = message
	choices := []map[string]json.RawMessage{}
	if len(c.choice) > 0 {
		choices = append(choices, c.choice)
	}
	encoded, err := json.Marshal(choices)
	if err != nil {
		return result, err
	}
	c.root["choices"] = encoded
	if c.usage != nil {
		c.root["usage"] = c.usage
	}
	response, err := json.Marshal(c.root)
	if err != nil {
		return result, err
	}
	result.Output, err = capturedOutput("stream_result", response, c.extensions)
	if result.Text != "" {
		result.Content = []model.Block{{Type: "text", Text: result.Text}}
	}
	return result, err
}

func mergeLogprobs(previous, next json.RawMessage) (json.RawMessage, error) {
	fields := make(map[string]json.RawMessage)
	if len(previous) > 0 && string(previous) != "null" {
		if err := json.Unmarshal(previous, &fields); err != nil {
			return nil, err
		}
	}
	var incoming map[string]json.RawMessage
	if err := json.Unmarshal(next, &incoming); err != nil {
		return nil, err
	}
	for key, value := range incoming {
		if (key == "content" || key == "refusal") && string(value) != "null" {
			var old, current []json.RawMessage
			if len(fields[key]) > 0 {
				if err := json.Unmarshal(fields[key], &old); err != nil {
					return nil, err
				}
			}
			if err := json.Unmarshal(value, &current); err != nil {
				return nil, err
			}
			encoded, err := json.Marshal(append(old, current...))
			if err != nil {
				return nil, err
			}
			fields[key] = encoded
		} else if _, exists := fields[key]; !exists || string(value) != "null" {
			fields[key] = value
		}
	}
	return json.Marshal(fields)
}
