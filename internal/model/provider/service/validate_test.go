package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/model/provider"
)

func TestInvalidModels(t *testing.T) {
	cases := []provider.ModelInput{
		{ProviderID: "01900000-0000-7000-8000-000000000001", DisplayName: "m", RemoteModelID: "r", Capabilities: map[string]bool{"reasoning": false}, ReasoningLevels: []provider.ReasoningLevel{{DisplayName: "中", RemoteValue: "medium"}}},
		{},
		{ProviderID: "01900000-0000-7000-8000-000000000001", DisplayName: "m", RemoteModelID: "r", ContextWindow: new(int64(0))},
		{ProviderID: "01900000-0000-7000-8000-000000000001", DisplayName: "m", RemoteModelID: "r", ModelType: []string{"reasoning"}},
		{ProviderID: "01900000-0000-7000-8000-000000000001", DisplayName: "m", RemoteModelID: "r", Capabilities: map[string]bool{"reasoning": true}, ModelType: []string{"reasoning", "reasoning"}},
		{ProviderID: "01900000-0000-7000-8000-000000000001", DisplayName: "m", RemoteModelID: "r", ModelType: []string{"text", "text"}},
		{ProviderID: "01900000-0000-7000-8000-000000000001", DisplayName: "m", RemoteModelID: "r", ModelType: []string{"image_input"}},
		{ProviderID: "01900000-0000-7000-8000-000000000001", DisplayName: "m", RemoteModelID: "r", ModelType: []string{"tools"}},
		{ProviderID: "01900000-0000-7000-8000-000000000001", DisplayName: "m", RemoteModelID: "r", ModelType: []string{"unknown"}},
		{ProviderID: "01900000-0000-7000-8000-000000000001", DisplayName: "m", RemoteModelID: "r", ReasoningLevels: []provider.ReasoningLevel{{DisplayName: "中", RemoteValue: "x"}, {DisplayName: "高", RemoteValue: "x"}}},
	}
	for _, in := range cases {
		if err := validateModel(in); !errors.Is(err, provider.ErrInvalidConfig) {
			t.Fatalf("未拒绝无效配置: %+v %v", in, err)
		}
	}
}

func TestContentTypesAreIndependentOfCapabilities(t *testing.T) {
	for _, capabilities := range []map[string]bool{nil, {}, {"tools": false}, {"reasoning": true}} {
		in := provider.ModelInput{ProviderID: testID, DisplayName: "模型", RemoteModelID: "remote", Capabilities: capabilities, ModelType: []string{"text", "image", "audio", "video"}}
		if err := validateModel(in); err != nil {
			t.Fatalf("内容类型被能力开关限制: %v", err)
		}
		in.ModelType = nil
		if err := validateModel(in); err != nil {
			t.Fatalf("未标注类型的配置被拒绝: %v", err)
		}
	}
}

func TestProviderValidationDoesNotExposeSecrets(t *testing.T) {
	p := provider.ProviderInput{Name: "p", Protocol: "chatcompletions", BaseURL: "https://secret:password@example.invalid", APIKey: new("private-key")}
	err := validateProvider(p)
	if !errors.Is(err, provider.ErrInvalidConfig) || strings.Contains(err.Error(), "password") || strings.Contains(err.Error(), "private-key") {
		t.Fatalf("错误未脱敏: %v", err)
	}
}
