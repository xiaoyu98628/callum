package migrations

import (
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	"gorm.io/gorm"
)

func catalogTables() []any {
	return []any{&schema.Provider{}, &schema.Model{}}
}

func validateCatalog(tx *gorm.DB) error {
	return validateUniqueIndex(tx, "models", "uq_provider_remote_model", []string{"provider_id", "remote_model_id"})
}
