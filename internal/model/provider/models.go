package provider

import "context"

// CreateModel 校验配置、分配 UUIDv7，并保存明确指定的启用状态。
func (s *Service) CreateModel(ctx context.Context, in ModelInput) (string, error) {
	if err := validateModel(in); err != nil {
		return "", err
	}
	id, err := newID()
	if err != nil {
		return "", err
	}
	if err := s.repository.CreateModel(ctx, id, in); err != nil {
		return "", err
	}
	return id, nil
}

// UpdateModel 替换完整配置，保留记录标识和创建时间，不创建缺失记录。
func (s *Service) UpdateModel(ctx context.Context, id string, in ModelInput) error {
	if !ValidID(id) {
		return ErrInvalidConfig
	}
	if err := validateModel(in); err != nil {
		return err
	}
	return s.repository.UpdateModel(ctx, id, in)
}

// GetModel 读取并校验配置，包含禁用记录。
func (s *Service) GetModel(ctx context.Context, id string) (Model, error) {
	if !ValidID(id) {
		return Model{}, ErrInvalidConfig
	}
	m, err := s.repository.GetModel(ctx, id)
	if err != nil {
		return Model{}, err
	}
	if err := validateModel(m.ModelInput); err != nil {
		return Model{}, err
	}
	return m, nil
}

// ListModels 返回经过校验的配置；selectableOnly 只筛选启用状态，不代替适配器能力检查。
func (s *Service) ListModels(ctx context.Context, selectableOnly bool) ([]Model, error) {
	models, err := s.repository.ListModels(ctx, selectableOnly)
	if err != nil {
		return nil, err
	}
	for _, m := range models {
		if err := validateModel(m.ModelInput); err != nil {
			return nil, err
		}
	}
	return models, nil
}

// SetModelEnabled 控制模型是否能用于新的运行配置解析。
func (s *Service) SetModelEnabled(ctx context.Context, id string, enabled bool) error {
	if !ValidID(id) {
		return ErrInvalidConfig
	}
	return s.repository.SetModelEnabled(ctx, id, enabled)
}

// DeleteModel 删除配置，不影响已经取得快照的执行。
func (s *Service) DeleteModel(ctx context.Context, id string) error {
	if !ValidID(id) {
		return ErrInvalidConfig
	}
	return s.repository.DeleteModel(ctx, id)
}

// Resolve 取得同一时刻的厂商和模型快照，校验模型配置并拒绝禁用模型。
// 返回值包含密钥，仅用于内部调用装配，不能整体记录或作为普通查询结果输出。
func (s *Service) Resolve(ctx context.Context, id string) (RuntimeConfig, error) {
	if !ValidID(id) {
		return RuntimeConfig{}, ErrInvalidConfig
	}
	runtime, err := s.repository.LoadRuntime(ctx, id)
	if err != nil {
		return RuntimeConfig{}, err
	}
	if err := validateModel(runtime.Model.ModelInput); err != nil {
		return RuntimeConfig{}, err
	}
	if !runtime.Model.Enabled {
		return RuntimeConfig{}, ErrDisabled
	}
	return runtime, nil
}
