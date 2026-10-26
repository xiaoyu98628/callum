package app

import (
	"context"
	"errors"
	"testing"

	"github.com/xiaoyu98628/callum/internal/config"
)

func TestRunAndClose(t *testing.T) {
	application, err := Compose(context.Background(), config.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := application.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := application.Close(); err != nil {
		t.Fatal(err)
	}
	if err := application.Close(); err != nil {
		t.Fatal(err)
	}
	if err := application.db.PingContext(context.Background()); err == nil {
		t.Fatal("database remained open after App.Close")
	}
}
