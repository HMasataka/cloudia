package config

import (
	"time"

	"github.com/spf13/viper"
)

// setDefaults はviperにデフォルト値を設定します。
func setDefaults(v *viper.Viper) {
	// Server
	v.SetDefault("server.host", "127.0.0.1")
	v.SetDefault("server.port", 4566)
	v.SetDefault("server.timeout", 30*time.Second)
	v.SetDefault("server.imds.enabled", false)
	v.SetDefault("server.imds.address", "169.254.169.254:80")

	// Logging
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.access_log", true)

	// Docker
	v.SetDefault("docker.api_version", "1.41")
	v.SetDefault("docker.network_name", "cloudia")
	v.SetDefault("docker.label_prefix", "cloudia")

	// Limits
	v.SetDefault("limits.max_containers", 20)
	v.SetDefault("limits.default_cpu", "1")
	v.SetDefault("limits.default_memory", "512m")
	v.SetDefault("limits.storage_quota", "10g")

	// State
	v.SetDefault("state.backend", "memory")
	v.SetDefault("state.file_path", "~/.cloudia/state.json")
	v.SetDefault("state.reconciliation_interval", 30*time.Second)
	v.SetDefault("state.lock_timeout", 30*time.Second)

	// Ports
	v.SetDefault("ports.range_start", 10000)
	v.SetDefault("ports.range_end", 20000)
	v.SetDefault("ports.max_per_resource", 10)

	// Cleanup
	v.SetDefault("cleanup.ttl_enabled", false)
	v.SetDefault("cleanup.default_ttl", 24*time.Hour)
	v.SetDefault("cleanup.orphan_check_interval", 5*time.Minute)

	// Metrics
	v.SetDefault("metrics.enabled", false)
	v.SetDefault("metrics.port", 9090)

	// Auth
	v.SetDefault("auth.mode", "local")
	v.SetDefault("auth.aws.access_key", "test")
	v.SetDefault("auth.aws.secret_key", "test")
	v.SetDefault("auth.aws.account_id", "000000000000")
	v.SetDefault("auth.aws.region", "us-east-1")
}
