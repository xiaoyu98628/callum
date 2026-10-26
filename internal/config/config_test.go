package config

import (
	"errors"
	"flag"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
)

func TestLoadPathsAndLevel(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data", "..", "callum")
	cfg, err := Load([]string{"--data-dir", dir, "--log-level", "debug"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != filepath.Clean(dir) || cfg.LogLevel != slog.LevelDebug {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.DatabasePath() != filepath.Join(cfg.DataDir, "callum.db") || cfg.LogPath() != filepath.Join(cfg.DataDir, "logs", "callum.log") {
		t.Fatalf("unexpected paths: %s %s", cfg.DatabasePath(), cfg.LogPath())
	}
	defaultConfig, err := Load(nil, io.Discard)
	if err != nil || !filepath.IsAbs(defaultConfig.DataDir) {
		t.Fatalf("default config: %+v, error: %v", defaultConfig, err)
	}
}

func TestLoadRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{{"--log-level", "unknown"}, {"--log-level", "error+1"}, {"--unknown"}, {"unexpected"}} {
		if _, err := Load(args, io.Discard); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	if _, err := Load([]string{"--help"}, io.Discard); !errors.Is(err, flag.ErrHelp) {
		t.Fatal(err)
	}
}
