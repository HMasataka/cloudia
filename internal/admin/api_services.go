package admin

import (
	"context"
	"net/http"

	"github.com/HMasataka/cloudia/internal/service"
)

// serviceInfo はサービス一覧 API の各エントリです。
type serviceInfo struct {
	Key      string             `json:"key"`
	Provider string             `json:"provider"`
	Name     string             `json:"name"`
	Health   service.HealthStatus `json:"health"`
}

// ListServicesHandler は GET /admin/api/services を処理します。
// Registry.ListServices と Registry.HealthAll を結合します。
func (h *Handler) ListServicesHandler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	var metas map[string]service.ServiceMeta
	var statuses map[string]service.HealthStatus

	if h.registry != nil {
		metas = h.registry.ListServices()
		statuses = h.registry.HealthAll(ctx)
	} else {
		metas = map[string]service.ServiceMeta{}
		statuses = map[string]service.HealthStatus{}
	}

	result := make([]serviceInfo, 0, len(metas))
	for key, meta := range metas {
		info := serviceInfo{
			Key:      key,
			Provider: meta.Provider,
			Name:     meta.Name,
			Health:   statuses[key],
		}
		result = append(result, info)
	}

	writeJSON(w, http.StatusOK, map[string][]serviceInfo{"data": result})
}
