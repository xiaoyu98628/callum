package provider

import (
	"fmt"
	"net/url"
	"strings"
)

func validateProvider(in ProviderInput) error {
	if strings.TrimSpace(in.Name) == "" {
		return fmt.Errorf("%w: 厂商名称不能为空", ErrInvalidConfig)
	}
	if in.Protocol != "chatcompletions" {
		return fmt.Errorf("%w: 当前不支持此协议", ErrInvalidConfig)
	}
	u, err := url.Parse(in.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%w: API 根地址无效", ErrInvalidConfig)
	}
	return nil
}

func validateModel(in ModelInput) error {
	if !ValidID(in.ProviderID) || strings.TrimSpace(in.DisplayName) == "" || strings.TrimSpace(in.RemoteModelID) == "" {
		return fmt.Errorf("%w: 厂商、名称和远端模型标识必须有效", ErrInvalidConfig)
	}
	if in.ContextWindow != nil && *in.ContextWindow <= 0 || in.MaxOutputTokens != nil && *in.MaxOutputTokens <= 0 {
		return fmt.Errorf("%w: Token 容量必须大于零", ErrInvalidConfig)
	}
	for feature := range in.Capabilities {
		if !knownFeature(feature) {
			return fmt.Errorf("%w: 未识别的模型功能", ErrInvalidConfig)
		}
	}
	selected := make(map[string]bool, len(in.ModelType))
	for _, feature := range in.ModelType {
		if !knownFeature(feature) || !in.Capabilities[feature] || selected[feature] {
			return fmt.Errorf("%w: 所选功能不受支持或重复", ErrInvalidConfig)
		}
		selected[feature] = true
	}
	levels := make(map[string]bool, len(in.ReasoningLevels))
	if supported, known := in.Capabilities["reasoning"]; known && !supported && len(in.ReasoningLevels) > 0 {
		return fmt.Errorf("%w: 不支持推理的模型不能配置推理等级", ErrInvalidConfig)
	}
	for _, level := range in.ReasoningLevels {
		if strings.TrimSpace(level.DisplayName) == "" || strings.TrimSpace(level.RemoteValue) == "" || levels[level.RemoteValue] {
			return fmt.Errorf("%w: 推理等级的名称或远端值无效", ErrInvalidConfig)
		}
		levels[level.RemoteValue] = true
	}
	return nil
}

func knownFeature(feature string) bool {
	switch feature {
	case "reasoning", "image_input", "tools":
		return true
	default:
		return false
	}
}
