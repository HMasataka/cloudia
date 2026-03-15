package admin

import (
	"net/http"
	"time"

	"github.com/HMasataka/cloudia/internal/config"
)

const masked = "***"

// maskedConfig は機密情報をマスクした設定の JSON シリアライズ用構造体です。
type maskedConfig struct {
	Server    config.ServerConfig    `json:"server"`
	Logging   config.LoggingConfig   `json:"logging"`
	Docker    config.DockerConfig    `json:"docker"`
	Limits    config.LimitsConfig    `json:"limits"`
	State     config.StateConfig     `json:"state"`
	Cleanup   config.CleanupConfig   `json:"cleanup"`
	Metrics   config.MetricsConfig   `json:"metrics"`
	Ports     config.PortConfig      `json:"ports"`
	Auth      maskedAuthConfig       `json:"auth"`
	Endpoints config.EndpointsConfig `json:"endpoints"`
}

type maskedAuthConfig struct {
	Mode string           `json:"mode"`
	AWS  maskedAWSConfig  `json:"aws"`
	GCP  maskedGCPConfig  `json:"gcp"`
}

type maskedAWSConfig struct {
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	AccountID string `json:"account_id"`
	Region    string `json:"region"`
}

type maskedGCPConfig struct {
	CredentialsFile string `json:"credentials_file"`
	Project         string `json:"project"`
	Zone            string `json:"zone"`
}

// serverConfigJSON は time.Duration を秒文字列に変換して JSON 出力するためのラッパーです。
type serverConfigJSON struct {
	Host    string        `json:"host"`
	Port    int           `json:"port"`
	Timeout time.Duration `json:"timeout"`
	IMDS    config.IMDSConfig `json:"imds"`
}

// GetConfigHandler は GET /admin/api/config を処理します。
// Auth.AWS.AccessKey, SecretKey, Auth.GCP.CredentialsFile を "***" にマスクします。
func (h *Handler) GetConfigHandler(w http.ResponseWriter, r *http.Request) {
	if h.config == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "config not available"})
		return
	}

	cfg := h.config

	mc := maskedConfig{
		Server:    cfg.Server,
		Logging:   cfg.Logging,
		Docker:    cfg.Docker,
		Limits:    cfg.Limits,
		State:     cfg.State,
		Cleanup:   cfg.Cleanup,
		Metrics:   cfg.Metrics,
		Ports:     cfg.Ports,
		Auth: maskedAuthConfig{
			Mode: cfg.Auth.Mode,
			AWS: maskedAWSConfig{
				AccessKey: masked,
				SecretKey: masked,
				AccountID: cfg.Auth.AWS.AccountID,
				Region:    cfg.Auth.AWS.Region,
			},
			GCP: maskedGCPConfig{
				CredentialsFile: masked,
				Project:         cfg.Auth.GCP.Project,
				Zone:            cfg.Auth.GCP.Zone,
			},
		},
		Endpoints: cfg.Endpoints,
	}

	writeJSON(w, http.StatusOK, mc)
}
