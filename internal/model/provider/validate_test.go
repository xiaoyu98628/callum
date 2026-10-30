package provider

import (
	"errors"
	"strings"
	"testing"
)

func TestInvalidModels(t *testing.T) {
	cases := []ModelInput{
		{ProviderID: "01900000-0000-7000-8000-000000000001", DisplayName: "m", RemoteModelID: "r", Capabilities: map[string]bool{"reasoning": false}, ReasoningLevels: []ReasoningLevel{{DisplayName: "中", RemoteValue: "medium"}}},
		{},
		{ProviderID: "01900000-0000-7000-8000-000000000001", DisplayName: "m", RemoteModelID: "r", ContextWindow: new(int64(0))},
		{ProviderID: "01900000-0000-7000-8000-000000000001", DisplayName: "m", RemoteModelID: "r", ModelType: []string{"reasoning"}},
		{ProviderID: "01900000-0000-7000-8000-000000000001", DisplayName: "m", RemoteModelID: "r", Capabilities: map[string]bool{"reasoning": true}, ModelType: []string{"reasoning", "reasoning"}},
		{ProviderID: "01900000-0000-7000-8000-000000000001", DisplayName: "m", RemoteModelID: "r", ReasoningLevels: []ReasoningLevel{{DisplayName: "中", RemoteValue: "x"}, {DisplayName: "高", RemoteValue: "x"}}},
	}
	for _, in := range cases {
		if err := validateModel(in); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("未拒绝无效配置: %+v %v", in, err)
		}
	}
}

func TestProviderValidationDoesNotExposeSecrets(t *testing.T) {
	p := ProviderInput{Name: "p", Protocol: "chatcompletions", BaseURL: "https://secret:password@example.invalid", APIKey: new("private-key")}
	err := validateProvider(p)
	if !errors.Is(err, ErrInvalidConfig) || strings.Contains(err.Error(), "password") || strings.Contains(err.Error(), "private-key") {
		t.Fatalf("错误未脱敏: %v", err)
	}
}
