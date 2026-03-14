package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Server defaults
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, "127.0.0.1")
	}
	if cfg.Server.Port != 4566 {
		t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, 4566)
	}
	if cfg.Server.Timeout != 30*time.Second {
		t.Errorf("Server.Timeout = %v, want %v", cfg.Server.Timeout, 30*time.Second)
	}

	// Logging defaults
	if cfg.Logging.Level != "info" {
		t.Errorf("Logging.Level = %q, want %q", cfg.Logging.Level, "info")
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("Logging.Format = %q, want %q", cfg.Logging.Format, "json")
	}
	if !cfg.Logging.AccessLog {
		t.Errorf("Logging.AccessLog = false, want true")
	}

	// Docker defaults
	if cfg.Docker.APIVersion != "1.41" {
		t.Errorf("Docker.APIVersion = %q, want %q", cfg.Docker.APIVersion, "1.41")
	}
	if cfg.Docker.NetworkName != "cloudia" {
		t.Errorf("Docker.NetworkName = %q, want %q", cfg.Docker.NetworkName, "cloudia")
	}
	if cfg.Docker.LabelPrefix != "cloudia" {
		t.Errorf("Docker.LabelPrefix = %q, want %q", cfg.Docker.LabelPrefix, "cloudia")
	}

	// Limits defaults
	if cfg.Limits.MaxContainers != 20 {
		t.Errorf("Limits.MaxContainers = %d, want %d", cfg.Limits.MaxContainers, 20)
	}
	if cfg.Limits.DefaultCPU != "1" {
		t.Errorf("Limits.DefaultCPU = %q, want %q", cfg.Limits.DefaultCPU, "1")
	}
	if cfg.Limits.DefaultMemory != "512m" {
		t.Errorf("Limits.DefaultMemory = %q, want %q", cfg.Limits.DefaultMemory, "512m")
	}
	if cfg.Limits.StorageQuota != "10g" {
		t.Errorf("Limits.StorageQuota = %q, want %q", cfg.Limits.StorageQuota, "10g")
	}

	// State defaults
	if cfg.State.Backend != "memory" {
		t.Errorf("State.Backend = %q, want %q", cfg.State.Backend, "memory")
	}
	if cfg.State.ReconciliationInterval != 30*time.Second {
		t.Errorf("State.ReconciliationInterval = %v, want %v", cfg.State.ReconciliationInterval, 30*time.Second)
	}
	if cfg.State.LockTimeout != 30*time.Second {
		t.Errorf("State.LockTimeout = %v, want %v", cfg.State.LockTimeout, 30*time.Second)
	}

	// Ports defaults
	if cfg.Ports.RangeStart != 10000 {
		t.Errorf("Ports.RangeStart = %d, want %d", cfg.Ports.RangeStart, 10000)
	}
	if cfg.Ports.RangeEnd != 20000 {
		t.Errorf("Ports.RangeEnd = %d, want %d", cfg.Ports.RangeEnd, 20000)
	}
	if cfg.Ports.MaxPerResource != 10 {
		t.Errorf("Ports.MaxPerResource = %d, want %d", cfg.Ports.MaxPerResource, 10)
	}

	// Cleanup defaults
	if cfg.Cleanup.TTLEnabled {
		t.Errorf("Cleanup.TTLEnabled = true, want false")
	}
	if cfg.Cleanup.DefaultTTL != 24*time.Hour {
		t.Errorf("Cleanup.DefaultTTL = %v, want %v", cfg.Cleanup.DefaultTTL, 24*time.Hour)
	}
	if cfg.Cleanup.OrphanCheckInterval != 5*time.Minute {
		t.Errorf("Cleanup.OrphanCheckInterval = %v, want %v", cfg.Cleanup.OrphanCheckInterval, 5*time.Minute)
	}

	// Metrics defaults
	if cfg.Metrics.Enabled {
		t.Errorf("Metrics.Enabled = true, want false")
	}
	if cfg.Metrics.Port != 9090 {
		t.Errorf("Metrics.Port = %d, want %d", cfg.Metrics.Port, 9090)
	}
}

func TestLoadFromYAMLFile(t *testing.T) {
	yamlContent := `
server:
  host: "0.0.0.0"
  port: 8080
  timeout: 60s
logging:
  level: "debug"
  format: "console"
  access_log: false
docker:
  api_version: "1.43"
  network_name: "mynet"
  label_prefix: "myapp"
limits:
  max_containers: 50
  default_cpu: "2"
  default_memory: "1g"
  storage_quota: "20g"
state:
  backend: "file"
  file_path: "/tmp/state.json"
  reconciliation_interval: 60s
  lock_timeout: 45s
ports:
  range_start: 30000
  range_end: 40000
  max_per_resource: 5
cleanup:
  ttl_enabled: true
  default_ttl: 48h
  orphan_check_interval: 10m
metrics:
  enabled: true
  port: 9191
`

	tmpFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("failed to write temp config file: %v", err)
	}

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, "0.0.0.0")
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, 8080)
	}
	if cfg.Server.Timeout != 60*time.Second {
		t.Errorf("Server.Timeout = %v, want %v", cfg.Server.Timeout, 60*time.Second)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Logging.Level = %q, want %q", cfg.Logging.Level, "debug")
	}
	if cfg.Logging.Format != "console" {
		t.Errorf("Logging.Format = %q, want %q", cfg.Logging.Format, "console")
	}
	if cfg.Logging.AccessLog {
		t.Errorf("Logging.AccessLog = true, want false")
	}
	if cfg.Docker.APIVersion != "1.43" {
		t.Errorf("Docker.APIVersion = %q, want %q", cfg.Docker.APIVersion, "1.43")
	}
	if cfg.Docker.NetworkName != "mynet" {
		t.Errorf("Docker.NetworkName = %q, want %q", cfg.Docker.NetworkName, "mynet")
	}
	if cfg.Limits.MaxContainers != 50 {
		t.Errorf("Limits.MaxContainers = %d, want %d", cfg.Limits.MaxContainers, 50)
	}
	if cfg.State.Backend != "file" {
		t.Errorf("State.Backend = %q, want %q", cfg.State.Backend, "file")
	}
	if cfg.State.ReconciliationInterval != 60*time.Second {
		t.Errorf("State.ReconciliationInterval = %v, want %v", cfg.State.ReconciliationInterval, 60*time.Second)
	}
	if cfg.State.LockTimeout != 45*time.Second {
		t.Errorf("State.LockTimeout = %v, want %v", cfg.State.LockTimeout, 45*time.Second)
	}
	if cfg.Ports.RangeStart != 30000 {
		t.Errorf("Ports.RangeStart = %d, want %d", cfg.Ports.RangeStart, 30000)
	}
	if cfg.Ports.RangeEnd != 40000 {
		t.Errorf("Ports.RangeEnd = %d, want %d", cfg.Ports.RangeEnd, 40000)
	}
	if cfg.Ports.MaxPerResource != 5 {
		t.Errorf("Ports.MaxPerResource = %d, want %d", cfg.Ports.MaxPerResource, 5)
	}
	if !cfg.Cleanup.TTLEnabled {
		t.Errorf("Cleanup.TTLEnabled = false, want true")
	}
	if cfg.Cleanup.DefaultTTL != 48*time.Hour {
		t.Errorf("Cleanup.DefaultTTL = %v, want %v", cfg.Cleanup.DefaultTTL, 48*time.Hour)
	}
	if cfg.Cleanup.OrphanCheckInterval != 10*time.Minute {
		t.Errorf("Cleanup.OrphanCheckInterval = %v, want %v", cfg.Cleanup.OrphanCheckInterval, 10*time.Minute)
	}
	if !cfg.Metrics.Enabled {
		t.Errorf("Metrics.Enabled = false, want true")
	}
	if cfg.Metrics.Port != 9191 {
		t.Errorf("Metrics.Port = %d, want %d", cfg.Metrics.Port, 9191)
	}
}

func TestLoadEnvVarOverride(t *testing.T) {
	t.Setenv("CLOUDIA_SERVER_PORT", "9999")
	t.Setenv("CLOUDIA_LOGGING_LEVEL", "warn")
	t.Setenv("CLOUDIA_METRICS_ENABLED", "true")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Port != 9999 {
		t.Errorf("Server.Port = %d, want 9999 (from env)", cfg.Server.Port)
	}
	if cfg.Logging.Level != "warn" {
		t.Errorf("Logging.Level = %q, want %q (from env)", cfg.Logging.Level, "warn")
	}
	if !cfg.Metrics.Enabled {
		t.Errorf("Metrics.Enabled = false, want true (from env)")
	}
	// Default値が残っていることも確認
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Server.Host = %q, want %q (default)", cfg.Server.Host, "127.0.0.1")
	}
}

func TestLoadEnvVarOverridesYAML(t *testing.T) {
	yamlContent := `
server:
  port: 8080
logging:
  level: "debug"
`
	tmpFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("failed to write temp config file: %v", err)
	}

	t.Setenv("CLOUDIA_SERVER_PORT", "9999")

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// 環境変数がYAMLより優先される
	if cfg.Server.Port != 9999 {
		t.Errorf("Server.Port = %d, want 9999 (env should override yaml)", cfg.Server.Port)
	}
	// YAMLの値は残る
	if cfg.Logging.Level != "debug" {
		t.Errorf("Logging.Level = %q, want %q (from yaml)", cfg.Logging.Level, "debug")
	}
}
