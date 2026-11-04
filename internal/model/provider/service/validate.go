package service

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/xiaoyu98628/callum/internal/model/provider"
)

func validateProvider(in provider.ProviderInput) error {
	if strings.TrimSpace(in.Name) == "" {
		return fmt.Errorf("%w: 厂商名称不能为空", provider.ErrInvalidConfig)
	}
	if in.Protocol != "chatcompletions" {
		return fmt.Errorf("%w: 当前不支持此协议", provider.ErrInvalidConfig)
	}
	u, err := url.Parse(in.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%w: API 根地址无效", provider.ErrInvalidConfig)
	}
	return nil
}

func validateModel(in provider.ModelInput) error {
	if !provider.ValidID(in.ProviderID) || strings.TrimSpace(in.DisplayName) == "" || strings.TrimSpace(in.RemoteModelID) == "" {
		return fmt.Errorf("%w: 厂商、名称和远端模型标识必须有效", provider.ErrInvalidConfig)
	}
	if in.ContextWindow != nil && *in.ContextWindow <= 0 || in.MaxOutputTokens != nil && *in.MaxOutputTokens <= 0 {
		return fmt.Errorf("%w: Token 容量必须大于零", provider.ErrInvalidConfig)
	}
	for feature := range in.Capabilities {
		if !knownFeature(feature) {
			return fmt.Errorf("%w: 未识别的模型功能", provider.ErrInvalidConfig)
		}
	}
	selected := make(map[string]bool, len(in.ModelType))
	for _, kind := range in.ModelType {
		if !provider.ValidModelType(kind) || selected[kind] {
			return fmt.Errorf("%w: 模型内容类型无效或重复", provider.ErrInvalidConfig)
		}
		selected[kind] = true
	}
	levels := make(map[string]bool, len(in.ReasoningLevels))
	if supported, known := in.Capabilities["reasoning"]; known && !supported && len(in.ReasoningLevels) > 0 {
		return fmt.Errorf("%w: 不支持推理的模型不能配置推理等级", provider.ErrInvalidConfig)
	}
	for _, level := range in.ReasoningLevels {
		if strings.TrimSpace(level.DisplayName) == "" || strings.TrimSpace(level.RemoteValue) == "" || levels[level.RemoteValue] {
			return fmt.Errorf("%w: 推理等级的名称或远端值无效", provider.ErrInvalidConfig)
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
