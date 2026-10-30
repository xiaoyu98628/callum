package provider

import "errors"

// Service 负责厂商和模型配置的业务校验、标识分配及启用规则。
// 存储由调用方注入并管理生命周期，业务包不依赖数据库驱动或 ORM。
type Service struct{ repository Repository }

// NewService 使用现有存储创建配置服务，不创建或关闭外部资源。
func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("配置存储不能为空")
	}
	return &Service{repository: repository}, nil
}
