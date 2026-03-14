package admin

import (
	"encoding/json"
	"net/http"

	"github.com/HMasataka/cloudia/internal/backend/docker"
	"go.uber.org/zap"
)

// Handler は管理 API のハンドラを保持します。
type Handler struct {
	dockerClient *docker.Client
	logger       *zap.Logger
}

// NewHandler は Handler のコンストラクタです。
func NewHandler(dockerClient *docker.Client, logger *zap.Logger) *Handler {
	return &Handler{dockerClient: dockerClient, logger: logger}
}

// ServicesHandler は登録済みサービスの一覧を返します。
func (h *Handler) ServicesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string][]string{"services": {}})
}
