package app

import (
	"context"
	"net/http"

	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/adapter/chatcompletions"
	"github.com/xiaoyu98628/callum/internal/model/provider"
)

// newModelResolver 将配置业务与具体适配器连接，不持有数据库查询或网络调用事务。
func newModelResolver(providers *provider.Service, client *http.Client) inference.Resolver {
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
