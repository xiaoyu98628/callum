package app

import (
	"context"
	"net/http"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/adapter/chatcompletions"
	providerservice "github.com/xiaoyu98628/callum/internal/model/provider/service"
)

// newModelResolver 将配置业务与具体适配器连接，不持有数据库查询或网络调用事务。
func newModelResolver(providers *providerservice.Service, client *http.Client) inference.Resolver {
	return func(ctx context.Context, id string) (model.Client, string, error) {
		runtime, err := providers.Resolve(ctx, id)
		if err != nil {
			return nil, "", err
		}
		if runtime.Protocol != "chatcompletions" || len(runtime.Model.ModelType) != 0 {
			return nil, "", inference.ErrUnsupported
		}
		adapter, err := chatcompletions.New(chatcompletions.Config{BaseURL: runtime.BaseURL, APIKey: runtime.APIKey}, client)
		return adapter, runtime.Model.RemoteModelID, err
	}
}

func newConversationResolver(providers *providerservice.Service, client *http.Client) agent.Resolver {
	return func(ctx context.Context, id string) (agent.Resolved, error) {
		runtime, err := providers.Resolve(ctx, id)
		if err != nil {
			return agent.Resolved{}, err
		}
		if runtime.Protocol != "chatcompletions" {
			return agent.Resolved{}, inference.ErrUnsupported
		}
		imageInput, tools := false, false
		for _, feature := range runtime.Model.ModelType {
			switch feature {
			case "image_input":
				imageInput = true
			case "tools":
				tools = true
			default:
				return agent.Resolved{}, inference.ErrUnsupported
			}
		}
		adapter, err := chatcompletions.New(chatcompletions.Config{BaseURL: runtime.BaseURL, APIKey: runtime.APIKey}, client)
		if err != nil {
			return agent.Resolved{}, err
		}
		return agent.Resolved{Client: adapter, Protocol: runtime.Protocol, ImageInput: imageInput, Tools: tools, Snapshot: model.Snapshot{ID: runtime.Model.ID, DisplayName: runtime.Model.DisplayName, RemoteModelID: runtime.Model.RemoteModelID, Provider: model.ProviderSnapshot{ID: runtime.Model.ProviderID, Name: runtime.ProviderName}}}, nil
	}
}
