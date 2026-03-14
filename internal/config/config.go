package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config はトップレベルの設定構造体です。
type Config struct {
	Server    ServerConfig    `mapstructure:"server"    yaml:"server"`
	Logging   LoggingConfig   `mapstructure:"logging"   yaml:"logging"`
	Docker    DockerConfig    `mapstructure:"docker"    yaml:"docker"`
	Limits    LimitsConfig    `mapstructure:"limits"    yaml:"limits"`
	State     StateConfig     `mapstructure:"state"     yaml:"state"`
	Cleanup   CleanupConfig   `mapstructure:"cleanup"   yaml:"cleanup"`
	Metrics   MetricsConfig   `mapstructure:"metrics"   yaml:"metrics"`
	Ports     PortConfig      `mapstructure:"ports"     yaml:"ports"`
	Auth      AuthConfig      `mapstructure:"auth"      yaml:"auth"`
	Endpoints EndpointsConfig `mapstructure:"endpoints" yaml:"endpoints"`
}

// AWSAuthConfig は AWS 認証の設定です。
type AWSAuthConfig struct {
	AccessKey string `mapstructure:"access_key" yaml:"access_key"`
	SecretKey string `mapstructure:"secret_key" yaml:"secret_key"`
}

// GCPAuthConfig は GCP 認証の設定です。
type GCPAuthConfig struct {
	CredentialsFile string `mapstructure:"credentials_file" yaml:"credentials_file"`
}

// AuthConfig は認証の設定です。
type AuthConfig struct {
	Mode string        `mapstructure:"mode" yaml:"mode"`
	AWS  AWSAuthConfig `mapstructure:"aws"  yaml:"aws"`
	GCP  GCPAuthConfig `mapstructure:"gcp"  yaml:"gcp"`
}

// ServiceEndpointConfig は個々のサービスエンドポイントの設定です。
type ServiceEndpointConfig struct {
	Port int `mapstructure:"port" yaml:"port"`
}

// EndpointsConfig はサービスエンドポイントの設定です。
type EndpointsConfig struct {
	Services map[string]ServiceEndpointConfig `mapstructure:"services" yaml:"services"`
}

// ServerConfig はHTTPサーバーの設定です。
type ServerConfig struct {
	Host    string        `mapstructure:"host"    yaml:"host"`
	Port    int           `mapstructure:"port"    yaml:"port"`
	Timeout time.Duration `mapstructure:"timeout" yaml:"timeout"`
}

// LoggingConfig はロガーの設定です。
type LoggingConfig struct {
	Level     string `mapstructure:"level"      yaml:"level"`
	Format    string `mapstructure:"format"     yaml:"format"`
	AccessLog bool   `mapstructure:"access_log" yaml:"access_log"`
}

// DockerConfig はDocker SDKクライアントの設定です。
type DockerConfig struct {
	APIVersion  string `mapstructure:"api_version"  yaml:"api_version"`
	NetworkName string `mapstructure:"network_name" yaml:"network_name"`
	LabelPrefix string `mapstructure:"label_prefix" yaml:"label_prefix"`
}

// LimitsConfig はリソース制限の設定です。
type LimitsConfig struct {
	MaxContainers int    `mapstructure:"max_containers"  yaml:"max_containers"`
	DefaultCPU    string `mapstructure:"default_cpu"     yaml:"default_cpu"`
	DefaultMemory string `mapstructure:"default_memory"  yaml:"default_memory"`
	StorageQuota  string `mapstructure:"storage_quota"   yaml:"storage_quota"`
}

// StateConfig は状態管理バックエンドの設定です。
type StateConfig struct {
	Backend                string        `mapstructure:"backend"                  yaml:"backend"`
	FilePath               string        `mapstructure:"file_path"                yaml:"file_path"`
	ReconciliationInterval time.Duration `mapstructure:"reconciliation_interval"  yaml:"reconciliation_interval"`
	LockTimeout            time.Duration `mapstructure:"lock_timeout"             yaml:"lock_timeout"`
}

// PortConfig はポート管理の設定です。
type PortConfig struct {
	RangeStart     int `mapstructure:"range_start"      yaml:"range_start"`
	RangeEnd       int `mapstructure:"range_end"        yaml:"range_end"`
	MaxPerResource int `mapstructure:"max_per_resource" yaml:"max_per_resource"`
}

// CleanupConfig はリソースのクリーンアップ設定です。
type CleanupConfig struct {
	TTLEnabled          bool          `mapstructure:"ttl_enabled"           yaml:"ttl_enabled"`
	DefaultTTL          time.Duration `mapstructure:"default_ttl"           yaml:"default_ttl"`
	OrphanCheckInterval time.Duration `mapstructure:"orphan_check_interval" yaml:"orphan_check_interval"`
}

// MetricsConfig はメトリクスサーバーの設定です。
type MetricsConfig struct {
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`
	Port    int  `mapstructure:"port"    yaml:"port"`
}

// Load は設定を読み込んで Config を返します。
// configPath が空でない場合はそのファイルを読み込みます。
// 空の場合は $HOME/.cloudia/config.yaml → ./configs/default.yaml の順で探索します。
func Load(configPath string) (*Config, error) {
	v := viper.New()

	setDefaults(v)

	v.SetConfigType("yaml")

	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("config: failed to read config file %q: %w", configPath, err)
		}
	} else {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			homeCfg := filepath.Join(homeDir, ".cloudia", "config.yaml")
			if _, statErr := os.Stat(homeCfg); statErr == nil {
				v.SetConfigFile(homeCfg)
				if readErr := v.ReadInConfig(); readErr != nil {
					return nil, fmt.Errorf("config: failed to read config file %q: %w", homeCfg, readErr)
				}
			}
		}

		if v.ConfigFileUsed() == "" {
			localCfg := filepath.Join("configs", "default.yaml")
			if _, statErr := os.Stat(localCfg); statErr == nil {
				v.SetConfigFile(localCfg)
				if readErr := v.ReadInConfig(); readErr != nil {
					return nil, fmt.Errorf("config: failed to read config file %q: %w", localCfg, readErr)
				}
			}
		}
	}

	v.SetEnvPrefix("CLOUDIA")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("config: failed to unmarshal config: %w", err)
	}

	return cfg, nil
}
