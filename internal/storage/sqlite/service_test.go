package sqlite_test

import (
	"github.com/xiaoyu98628/callum/internal/model/provider"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	"gorm.io/gorm"
)

func newService(db *gorm.DB) (*provider.Service, error) {
	repository, err := sqlite.NewCatalogRepository(db)
	if err != nil {
		return nil, err
	}
	return provider.NewService(repository)
}
