package admin

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/HMasataka/cloudia/internal/backend/docker"
	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/state"
	"go.uber.org/zap"
)

// Handler は管理 API のハンドラを保持します。
type Handler struct {
	dockerClient *docker.Client
	store        state.Store
	registry     *service.Registry
	config       *config.Config
	logger       *zap.Logger
}

// NewHandler は Handler のコンストラクタです。
func NewHandler(dockerClient *docker.Client, store state.Store, registry *service.Registry, cfg *config.Config, logger *zap.Logger) *Handler {
	return &Handler{
		dockerClient: dockerClient,
		store:        store,
		registry:     registry,
		config:       cfg,
		logger:       logger,
	}
}

// ServicesHandler は登録済みサービスの一覧を返します。
func (h *Handler) ServicesHandler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	var statuses map[string]service.HealthStatus
	if h.registry != nil {
		statuses = h.registry.HealthAll(ctx)
	} else {
		statuses = map[string]service.HealthStatus{}
	}

	services := make([]string, 0, len(statuses))
	for key := range statuses {
		services = append(services, key)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string][]string{"services": services}) //nolint:errcheck
}
