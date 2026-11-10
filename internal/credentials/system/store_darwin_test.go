//go:build darwin

package system

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"slices"
	"testing"
	"time"
)

func TestIsolatedSystemKeychainRoundTrip(t *testing.T) {
	if os.Getenv("CALLUM_LIVE_KEYRING_TEST") != "1" {
		t.Skip("系统凭据库实测需显式启用；默认测试使用隔离替身")
	}
	digest := sha256.Sum256([]byte("callum-credential-test-" + rand.Text()))
	name := hex.EncodeToString(digest[:])
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	defer clear(key)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	// 只删除本次随机命名的测试条目，不枚举或读取其他用户凭据。
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
		defer stop()
		if err := exec.CommandContext(cleanup, "/usr/bin/security", "delete-generic-password", "-s", service, "-a", name).Run(); err != nil {
			t.Error("清理测试钥匙串条目失败", err)
		}
	})
	store := New()
	if err := store.Save(ctx, name, key); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(ctx, name)
	defer clear(got)
	if err != nil || !slices.Equal(got, key) {
		t.Fatal("系统凭据读写不一致", err)
	}
}
