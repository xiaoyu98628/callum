package execution

import "github.com/xiaoyu98628/callum/internal/model"

type invokeParams struct {
	ModelID  string `json:"model_id"`
	Stream   *bool  `json:"stream"`
	Messages []struct {
		Role    model.Role `json:"role"`
		Content *string    `json:"content"`
	} `json:"messages"`
}

type cancelParams struct {
	RunID string `json:"run_id"`
}
