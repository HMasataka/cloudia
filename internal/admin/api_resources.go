package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
)

const (
	defaultPerPage = 50
	maxPerPage     = 200
)

// resourcesListResponse はリソース一覧 API のレスポンス形式です。
type resourcesListResponse struct {
	Data    []*models.Resource `json:"data"`
	Page    int                `json:"page"`
	PerPage int                `json:"per_page"`
	Total   int                `json:"total"`
}

// ListResourcesHandler は GET /admin/api/resources を処理します。
// query: provider, service, kind, page, per_page
func (h *Handler) ListResourcesHandler(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store not available"})
		return
	}

	ctx := r.Context()

	q := r.URL.Query()
	provider := q.Get("provider")
	svc := q.Get("service")
	kind := q.Get("kind")

	page := 1
	if p := q.Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}

	perPage := defaultPerPage
	if pp := q.Get("per_page"); pp != "" {
		if v, err := strconv.Atoi(pp); err == nil && v > 0 {
			perPage = v
		}
	}
	if perPage > maxPerPage {
		perPage = maxPerPage
	}

	filter := state.Filter{}
	if provider != "" {
		filter["Provider"] = provider
	}
	if svc != "" {
		filter["Service"] = svc
	}

	resources, err := h.store.List(ctx, kind, filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	total := len(resources)

	// アプリ層ページネーション
	start := (page - 1) * perPage
	if start >= total {
		resources = []*models.Resource{}
	} else {
		end := start + perPage
		if end > total {
			end = total
		}
		resources = resources[start:end]
	}

	writeJSON(w, http.StatusOK, resourcesListResponse{
		Data:    resources,
		Page:    page,
		PerPage: perPage,
		Total:   total,
	})
}

// GetResourceHandler は GET /admin/api/resources/{kind}/{id} を処理します。
func (h *Handler) GetResourceHandler(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store not available"})
		return
	}

	ctx := r.Context()

	kind := r.PathValue("kind")
	id := r.PathValue("id")

	resource, err := h.store.Get(ctx, kind, id)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "resource not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, resource)
}

// DeleteResourceHandler は DELETE /admin/api/resources/{kind}/{id} を処理します。
// ContainerID があれば先に docker.StopContainer → docker.RemoveContainer を実行してから Store.Delete します。
func (h *Handler) DeleteResourceHandler(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store not available"})
		return
	}

	ctx := r.Context()

	kind := r.PathValue("kind")
	id := r.PathValue("id")

	resource, err := h.store.Get(ctx, kind, id)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "resource not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Docker コンテナが存在する場合は先に停止・削除する（孤児コンテナ防止）
	if resource.ContainerID != "" && h.dockerClient != nil {
		if err := h.dockerClient.StopContainer(ctx, resource.ContainerID, nil); err != nil {
			h.logger.Sugar().Warnf("stop container %s: %v", resource.ContainerID, err)
		}
		if err := h.dockerClient.RemoveContainer(ctx, resource.ContainerID); err != nil {
			h.logger.Sugar().Warnf("remove container %s: %v", resource.ContainerID, err)
		}
	}

	if err := h.store.Delete(ctx, kind, id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// writeJSON は JSON レスポンスを書き込む共通ヘルパーです。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}
