package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/xiaoyu98628/callum/internal/credentials"
	domain "github.com/xiaoyu98628/callum/internal/mcp"
	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository 保存 MCP 配置及密文。加解密和网络请求均在写事务之外。
type Repository struct {
	db     *gorm.DB
	cipher credentials.Cipher
}

func New(db *gorm.DB, cipher credentials.Cipher) *Repository {
	return &Repository{db: db, cipher: cipher}
}

func (r *Repository) Load(ctx context.Context) (settings.MCP, int64, error) {
	var row schema.Settings
	if err := r.db.WithContext(ctx).Where("key = ?", settings.MCPKey).Take(&row).Error; err != nil {
		return settings.MCP{}, 0, err
	}
	cfg, err := settings.DecodeMCPRecord(settings.Record{Key: row.Key, Value: row.Value, Revision: row.Revision})
	return cfg, row.Revision, err
}

func (r *Repository) Credential(ctx context.Context, id string) (domain.Credential, error) {
	var row schema.MCPCredential
	err := r.db.WithContext(ctx).Where("server_id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Credential{}, nil
	}
	if err != nil {
		return domain.Credential{}, err
	}
	plain, err := r.cipher.Open(ctx, "mcp/"+id, row.Ciphertext)
	if err != nil {
		return domain.Credential{}, err
	}
	var value domain.Credential
	err = json.Unmarshal([]byte(plain), &value)
	return value, err
}

func (r *Repository) Save(ctx context.Context, cfg settings.MCP, rev int64, id string, credential *domain.Credential) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if _, err := settings.DecodeMCP(raw); err != nil {
		return err
	}
	sealed, err := r.seal(ctx, id, credential)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&schema.Settings{}).Where("key = ? AND revision = ?", settings.MCPKey, rev).
			Updates(map[string]any{"value": string(raw), "revision": rev + 1, "updated_at": time.Now().UnixMilli()})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return settings.ErrConflict
		}
		if credential == nil {
			return nil
		}
		return writeCredential(tx, id, sealed)
	})
}

func (r *Repository) SaveCredential(ctx context.Context, id string, credential domain.Credential) error {
	sealed, err := r.seal(ctx, id, &credential)
	if err != nil {
		return err
	}
	return writeCredential(r.db.WithContext(ctx), id, sealed)
}

func (r *Repository) seal(ctx context.Context, id string, value *domain.Credential) (string, error) {
	if value == nil || value.Empty() {
		return "", nil
	}
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return r.cipher.Seal(ctx, "mcp/"+id, string(b))
}

func writeCredential(tx *gorm.DB, id, sealed string) error {
	if sealed == "" {
		return tx.Where("server_id = ?", id).Delete(&schema.MCPCredential{}).Error
	}
	row := schema.MCPCredential{ServerID: id, Ciphertext: sealed, UpdatedAt: time.Now().UnixMilli()}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "server_id"}}, DoUpdates: clause.AssignmentColumns([]string{"ciphertext", "updated_at"})}).Create(&row).Error
}
