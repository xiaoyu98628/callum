package credentials

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

type memoryKeyring struct {
	key   []byte
	fail  bool
	saves int
}

func (m *memoryKeyring) Load(context.Context, string) ([]byte, error) {
	if m.fail {
		return nil, ErrUnavailable
	}
	if m.key == nil {
		return nil, ErrNotFound
	}
	return append([]byte(nil), m.key...), nil
}
func (m *memoryKeyring) Save(_ context.Context, _ string, key []byte) error {
	m.saves++
	m.key = append([]byte(nil), key...)
	return nil
}

func TestVaultAuthenticationPersistenceAndClosure(t *testing.T) {
	store := &memoryKeyring{}
	v, err := New(store, "temporary database")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { v.Close() })
	first, err := v.Seal(t.Context(), "provider-a", "private-value")
	if err != nil || strings.Contains(first, "private-value") {
		t.Fatal("凭据未加密", err)
	}
	second, err := v.Seal(t.Context(), "provider-a", "private-value")
	if err != nil || first == second || store.saves != 1 {
		t.Fatal("nonce 或主密钥生命周期错误", err)
	}
	reopened, _ := New(store, "temporary database")
	defer reopened.Close()
	if value, err := reopened.Open(t.Context(), "provider-a", first); err != nil || value != "private-value" {
		t.Fatal("无法恢复凭据", err)
	}
	parts := strings.Split(first, ":")
	changed, _ := base64.RawStdEncoding.DecodeString(parts[2])
	changed[0] ^= 1
	tampered := parts[0] + ":" + parts[1] + ":" + base64.RawStdEncoding.EncodeToString(changed)
	for _, tc := range []struct{ id, value string }{{"provider-b", first}, {"provider-a", tampered}, {"provider-a", "%%%"}} {
		if _, err := v.Open(t.Context(), tc.id, tc.value); !errors.Is(err, ErrInvalid) {
			t.Fatal("接受被替换的密文", err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := v.Seal(ctx, "id", "secret"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := v.Close(); err != nil {
		t.Fatal(err)
	}
	if err := v.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Open(t.Context(), "provider-a", first); !errors.Is(err, ErrUnavailable) {
		t.Fatal("关闭后仍使用主密钥", err)
	}
}

func TestMissingOrLockedKeyNeverCreatesReplacementWhileDecrypting(t *testing.T) {
	for _, fail := range []bool{false, true} {
		store := &memoryKeyring{fail: fail}
		v, _ := New(store, "temporary database")
		if _, err := v.Open(t.Context(), "provider", "v1:"+v.name+":"+strings.Repeat("A", 64)); !errors.Is(err, ErrUnavailable) || store.saves != 0 {
			t.Fatal("解密时创建了替代主密钥", err)
		}
		v.Close()
	}
}

func TestMovingDatabaseRetainsOriginalKeyReference(t *testing.T) {
	store := &memoryKeyring{}
	original, _ := New(store, "original database path")
	value, err := original.Seal(t.Context(), "provider", "private-value")
	if err != nil {
		t.Fatal(err)
	}
	original.Close()
	moved, _ := New(store, "moved database path")
	text, err := moved.Open(t.Context(), "provider", value)
	if err != nil || text != "private-value" || store.saves != 1 {
		t.Fatal("移动数据目录后丢失原凭据", err)
	}
	moved.Close()
	if _, err := moved.Open(t.Context(), "provider", value); !errors.Is(err, ErrUnavailable) {
		t.Fatal("关闭后仍加载旧目录主密钥", err)
	}
}
