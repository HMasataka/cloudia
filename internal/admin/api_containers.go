package admin

import (
	"net/http"
	"strconv"
)

const defaultLogLines = 100

// ListContainersHandler は GET /admin/api/containers を処理します。
func (h *Handler) ListContainersHandler(w http.ResponseWriter, r *http.Request) {
	if h.dockerClient == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "docker client not available"})
		return
	}

	containers, err := h.dockerClient.ListManagedContainers(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": containers})
}

// ContainerLogsHandler は GET /admin/api/containers/{id}/logs を処理します。
// query: lines（デフォルト 100）
func (h *Handler) ContainerLogsHandler(w http.ResponseWriter, r *http.Request) {
	containerID := r.PathValue("id")

	lines := defaultLogLines
	if l := r.URL.Query().Get("lines"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			lines = v
		}
	}

	if h.dockerClient == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "docker client not available"})
		return
	}

	logs, err := h.dockerClient.ContainerLogs(r.Context(), containerID, lines)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"logs": logs})
}
