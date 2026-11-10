// Package credentials 保护持久化凭据，明文仅在实际模型调用装配时解密。
package credentials

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
)

var (
	ErrNotFound    = errors.New("系统凭据不存在")
	ErrUnavailable = errors.New("系统凭据库不可用或已锁定")
	ErrInvalid     = errors.New("加密凭据无效或无法解密")
)

// MaxBytes 与配置请求字节边界一致，不把系统主密钥条目的大小限制施加给 API Key。
const MaxBytes = 1 << 20

// Keyring 只保存每个数据目录的随机主密钥，不能将其与数据库一起明文落盘。
// 实现负责调用范围、取消、输出边界和系统资源清理，不得记录密钥。
type Keyring interface {
	Load(context.Context, string) ([]byte, error)
	Save(context.Context, string, []byte) error
}

// Cipher 由装配层提供给配置仓储；Prepare 仅在有旧凭据迁移时于事务外调用。
type Cipher interface {
	Prepare(context.Context) error
	Seal(context.Context, string, string) (string, error)
	Open(context.Context, string, string) (string, error)
}

// Vault 缓存一个数据目录的主密钥；持有者在所有调用结束后 Close。
type Vault struct {
	mu      sync.Mutex
	keyring Keyring
	name    string
	key     []byte
	closed  bool
}

func New(keyring Keyring, identity string) (*Vault, error) {
	if keyring == nil || identity == "" {
		return nil, ErrInvalid
	}
	hash := sha256.Sum256([]byte(identity))
	return &Vault{keyring: keyring, name: hex.EncodeToString(hash[:])}, nil
}

func (v *Vault) master(ctx context.Context, create bool) ([]byte, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if v.closed {
		return nil, ErrUnavailable
	}
	if v.key == nil {
		key, err := v.keyring.Load(ctx, v.name)
		if errors.Is(err, ErrNotFound) && create {
			key = make([]byte, 32)
			if _, err := rand.Read(key); err != nil {
				return nil, err
			}
			if err := v.keyring.Save(ctx, v.name, key); err != nil {
				clear(key)
				return nil, errors.Join(ErrUnavailable, err, ctx.Err())
			}
			// 系统调用成功不替代写入核对；失败不能让数据库使用一个未安全保存的密钥。
			stored, readErr := v.keyring.Load(ctx, v.name)
			if readErr != nil || subtle.ConstantTimeCompare(key, stored) != 1 {
				clear(key)
				clear(stored)
				return nil, errors.Join(ErrUnavailable, readErr, ctx.Err())
			}
			clear(stored)
		} else if err != nil {
			clear(key)
			return nil, errors.Join(ErrUnavailable, err, ctx.Err())
		}
		if len(key) != 32 {
			clear(key)
			return nil, ErrInvalid
		}
		v.key = key
	}
	return append([]byte(nil), v.key...), nil
}

func (v *Vault) Prepare(ctx context.Context) error {
	key, err := v.master(ctx, true)
	clear(key)
	return err
}

func (v *Vault) aead(ctx context.Context, create bool) (cipher.AEAD, error) {
	key, err := v.master(ctx, create)
	if err != nil {
		return nil, err
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrInvalid
	}
	return cipher.NewGCMWithRandomNonce(block)
}

func (v *Vault) Seal(ctx context.Context, id, text string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if text == "" {
		return "", nil
	}
	if len(text) > MaxBytes {
		return "", ErrInvalid
	}
	aead, err := v.aead(ctx, true)
	if err != nil {
		return "", err
	}
	return "v1:" + v.name + ":" + base64.RawStdEncoding.EncodeToString(aead.Seal(nil, nil, []byte(text), []byte("callum/provider/v1/"+v.name+"/"+id))), nil
}

func (v *Vault) Open(ctx context.Context, id, encoded string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if encoded == "" {
		return "", nil
	}
	if len(encoded) > 2*MaxBytes {
		return "", ErrInvalid
	}
	parts := strings.Split(encoded, ":")
	if len(parts) != 3 || parts[0] != "v1" || len(parts[1]) != 64 {
		return "", ErrInvalid
	}
	if _, err := hex.DecodeString(parts[1]); err != nil {
		return "", ErrInvalid
	}
	raw, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(raw) > MaxBytes+28 {
		return "", ErrInvalid
	}
	var aead cipher.AEAD
	if parts[1] == v.name {
		aead, err = v.aead(ctx, false)
	} else {
		// 密文携带不含秘密的主密钥标识，同一系统用户移动数据目录后仍能读取。
		v.mu.Lock()
		if v.closed {
			v.mu.Unlock()
			return "", ErrUnavailable
		}
		key, keyErr := v.keyring.Load(ctx, parts[1])
		v.mu.Unlock()
		if keyErr != nil {
			clear(key)
			return "", errors.Join(ErrUnavailable, keyErr, ctx.Err())
		}
		defer clear(key)
		if len(key) != 32 {
			return "", ErrInvalid
		}
		block, keyErr := aes.NewCipher(key)
		if keyErr != nil {
			return "", ErrInvalid
		}
		aead, err = cipher.NewGCMWithRandomNonce(block)
	}
	if err != nil {
		return "", err
	}
	text, err := aead.Open(nil, nil, raw, []byte("callum/provider/v1/"+parts[1]+"/"+id))
	if err != nil {
		return "", ErrInvalid
	}
	defer clear(text)
	return string(text), nil
}

func (v *Vault) Close() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	clear(v.key)
	v.key, v.closed = nil, true
	return nil
}
