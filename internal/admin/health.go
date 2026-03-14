package admin

import (
	"encoding/json"
	"net/http"
)

// HealthHandler はサービスのヘルスチェックを返します。
func (h *Handler) HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
