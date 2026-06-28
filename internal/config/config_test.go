package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"gopherledger/internal/config"
)

func TestLoad_Defaults(t *testing.T) {
	// Временно переходим в пустую директорию
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.ServerHost != "localhost" {
		t.Errorf("expected localhost, got %s", cfg.ServerHost)
	}
	if cfg.ServerPort != 8080 {
		t.Errorf("expected 8080, got %d", cfg.ServerPort)
	}
}

func TestLoad_FromFile(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	content := `
server_host: "0.0.0.0"
server_port: 9090
log_level: "debug"
accrual_interval_seconds: 5
worker_concurrency: 10
`
	os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(content), 0644)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.ServerHost != "0.0.0.0" {
		t.Errorf("expected 0.0.0.0, got %s", cfg.ServerHost)
	}
	if cfg.ServerPort != 9090 {
		t.Errorf("expected 9090, got %d", cfg.ServerPort)
	}
	if cfg.WorkerConcurrency != 10 {
		t.Errorf("expected 10, got %d", cfg.WorkerConcurrency)
	}
}
