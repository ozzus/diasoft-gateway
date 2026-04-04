package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFallsBackToEnvWhenDefaultConfigIsAbsent(t *testing.T) {
	t.Setenv("CONFIG_PATH", "")
	t.Setenv("ENV", "test")
	t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/diasoft_gateway?sslmode=disable")
	t.Setenv("REDIS_ADDR", "localhost:6379")
	t.Setenv("HTTP_ADDRESS", ":18080")

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tempdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Database.URL != "postgres://postgres:postgres@localhost:5432/diasoft_gateway?sslmode=disable" {
		t.Fatalf("unexpected database url: %s", cfg.Database.URL)
	}
	if cfg.HTTP.Address != ":18080" {
		t.Fatalf("unexpected http address: %s", cfg.HTTP.Address)
	}
}

func TestLoadReturnsErrorForExplicitMissingConfig(t *testing.T) {
	t.Setenv("CONFIG_PATH", filepath.Join(t.TempDir(), "missing.yaml"))
	t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/diasoft_gateway?sslmode=disable")
	t.Setenv("REDIS_ADDR", "localhost:6379")

	if _, err := Load(); err == nil {
		t.Fatal("expected explicit missing config path to fail")
	}
}
