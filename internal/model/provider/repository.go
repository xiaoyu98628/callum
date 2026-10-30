package provider

import "context"

// Repository 定义配置业务所需的持久化操作，由具体存储包实现。
// 创建记录的标识由 Service 提供；更新不能创建缺失记录。
// 查询不得返回明文凭据，只有 LoadRuntime 可读取用于内部装配的凭据快照。
// 实现必须传递上下文，返回前释放查询占用，不能将事务带入后续模型调用。
type Repository interface {
	CreateProvider(context.Context, string, ProviderInput) error
	UpdateProvider(context.Context, string, ProviderInput) error
	GetProvider(context.Context, string) (Provider, error)
	ListProviders(context.Context) ([]Provider, error)
	DeleteProvider(context.Context, string) error
	CreateModel(context.Context, string, ModelInput) error
	UpdateModel(context.Context, string, ModelInput) error
	GetModel(context.Context, string) (Model, error)
	ListModels(context.Context, bool) ([]Model, error)
	SetModelEnabled(context.Context, string, bool) error
	DeleteModel(context.Context, string) error
	LoadRuntime(context.Context, string) (RuntimeConfig, error)
}
