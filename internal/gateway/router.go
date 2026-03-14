package gateway

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/HMasataka/cloudia/internal/admin"
	"github.com/HMasataka/cloudia/internal/gateway/middleware"
	"go.uber.org/zap"
)

// NewRouter は HTTP ルーターを構築して返します。
// ミドルウェアチェイン: recovery → logging → timeout → mux の順にラップします。
func NewRouter(adminHandler *admin.Handler, logger *zap.Logger, timeout time.Duration) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", adminHandler.HealthHandler)
	mux.HandleFunc("GET /admin/services", adminHandler.ServicesHandler)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "unsupported operation"})
	})

	var handler http.Handler = mux
	handler = middleware.Timeout(timeout)(handler)
	handler = middleware.Logging(logger)(handler)
	handler = middleware.Recovery(logger)(handler)

	return handler
}

// detectProvider はリクエストからプロバイダーを検出します。
// v0.1 では常に空文字列を返すスタブです。
func detectProvider(r *http.Request) string {
	return ""
}
