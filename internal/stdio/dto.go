package stdio

import (
	"bytes"
	"encoding/json"

	"github.com/xiaoyu98628/callum/internal/model/provider"
)

type providerFields struct {
	Name     string          `json:"name"`
	Protocol string          `json:"protocol"`
	BaseURL  string          `json:"base_url"`
	APIKey   json.RawMessage `json:"api_key"`
}

func (p providerFields) input() (provider.ProviderInput, *rpcError) {
	in := provider.ProviderInput{Name: p.Name, Protocol: p.Protocol, BaseURL: p.BaseURL}
	if p.APIKey != nil {
		var key *string
		if json.Unmarshal(p.APIKey, &key) != nil || key == nil {
			return in, invalidParams()
		}
		in.APIKey = key
	}
	return in, nil
}

type providerView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Protocol  string `json:"protocol"`
	BaseURL   string `json:"base_url"`
	HasAPIKey bool   `json:"has_api_key"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

func viewProvider(p provider.Provider) providerView {
	return providerView{p.ID, p.Name, p.Protocol, p.BaseURL, p.HasAPIKey, p.CreatedAt, p.UpdatedAt}
}

type modelFields struct {
	ProviderID      string                    `json:"provider_id"`
	DisplayName     string                    `json:"display_name"`
	RemoteModelID   string                    `json:"remote_model_id"`
	Enabled         *bool                     `json:"enabled"`
	ContextWindow   *int64                    `json:"context_window"`
	MaxOutputTokens *int64                    `json:"max_output_tokens"`
	Capabilities    json.RawMessage           `json:"capabilities"`
	ModelType       []string                  `json:"model_type"`
	ReasoningLevels []provider.ReasoningLevel `json:"reasoning_levels"`
}

func (p modelFields) input() (provider.ModelInput, *rpcError) {
	in := provider.ModelInput{ProviderID: p.ProviderID, DisplayName: p.DisplayName, RemoteModelID: p.RemoteModelID,
		ContextWindow: p.ContextWindow, MaxOutputTokens: p.MaxOutputTokens, ModelType: p.ModelType, ReasoningLevels: p.ReasoningLevels}
	if p.Enabled == nil {
		return in, invalidParams()
	}
	in.Enabled = *p.Enabled
	if p.Capabilities != nil && !bytes.Equal(bytes.TrimSpace(p.Capabilities), []byte("null")) {
		var fields map[string]json.RawMessage
		if json.Unmarshal(p.Capabilities, &fields) != nil || fields == nil {
			return in, invalidParams()
		}
		in.Capabilities = make(map[string]bool, len(fields))
		for k, raw := range fields {
			var value *bool
			if json.Unmarshal(raw, &value) != nil || value == nil {
				return in, invalidParams()
			}
			in.Capabilities[k] = *value
		}
	}
	return in, nil
}

type modelView struct {
	ID              string                    `json:"id"`
	ProviderID      string                    `json:"provider_id"`
	DisplayName     string                    `json:"display_name"`
	RemoteModelID   string                    `json:"remote_model_id"`
	Enabled         bool                      `json:"enabled"`
	ContextWindow   *int64                    `json:"context_window"`
	MaxOutputTokens *int64                    `json:"max_output_tokens"`
	Capabilities    map[string]bool           `json:"capabilities"`
	ModelType       []string                  `json:"model_type"`
	ReasoningLevels []provider.ReasoningLevel `json:"reasoning_levels"`
	CreatedAt       int64                     `json:"created_at"`
	UpdatedAt       int64                     `json:"updated_at"`
}

func viewModel(m provider.Model) modelView {
	return modelView{m.ID, m.ProviderID, m.DisplayName, m.RemoteModelID, m.Enabled, m.ContextWindow,
		m.MaxOutputTokens, m.Capabilities, m.ModelType, m.ReasoningLevels, m.CreatedAt, m.UpdatedAt}
}

type idParams struct {
	ID string `json:"id"`
}
type idResult struct {
	ID string `json:"id"`
}
