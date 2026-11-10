package catalog_test

import (
	"github.com/xiaoyu98628/callum/internal/credentials"
	"github.com/xiaoyu98628/callum/internal/credentials/credentialstest"
	providerservice "github.com/xiaoyu98628/callum/internal/model/provider/service"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/catalog"
	"gorm.io/gorm"
)

func newService(db *gorm.DB) (*providerservice.Service, error) {
	cipher, err := credentials.New(credentialstest.Keyring{}, "catalog test database")
	if err != nil {
		return nil, err
	}
	repository, err := catalog.NewRepository(db, cipher)
	if err != nil {
		return nil, err
	}
	return providerservice.New(repository)
}
