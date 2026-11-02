package catalog_test

import (
	providerservice "github.com/xiaoyu98628/callum/internal/model/provider/service"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/catalog"
	"gorm.io/gorm"
)

func newService(db *gorm.DB) (*providerservice.Service, error) {
	repository, err := catalog.NewRepository(db)
	if err != nil {
		return nil, err
	}
	return providerservice.New(repository)
}
