package gateway

import (
	"net/http"
	"time"

	"github.com/HMasataka/cloudia/internal/admin"
	"github.com/HMasataka/cloudia/internal/gateway/middleware"
	"go.uber.org/zap"
)

// NewRouter は HTTP ルーターを構築して返します。
// ミドルウェアチェイン: recovery → logging → idempotency → timeout → metrics → mux の順にラップします。
func NewRouter(adminHandler *admin.Handler, serviceHandler *ServiceHandler, logger *zap.Logger, timeout time.Duration) http.Handler {
	return NewRouterWithIdempotency(adminHandler, serviceHandler, logger, timeout, middleware.NewIdempotencyStore())
}

// NewRouterWithIdempotency は冪等性ストアを受け取って HTTP ルーターを構築して返します。
func NewRouterWithIdempotency(adminHandler *admin.Handler, serviceHandler *ServiceHandler, logger *zap.Logger, timeout time.Duration, idempStore *middleware.IdempotencyStore) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", adminHandler.HealthHandler)
	mux.HandleFunc("GET /admin/services", adminHandler.ServicesHandler)
	mux.Handle("/", serviceHandler)

	var handler http.Handler = mux
	handler = middleware.Timeout(timeout)(handler)
	handler = middleware.Metrics()(handler)
	handler = middleware.Idempotency(idempStore)(handler)
	handler = middleware.Logging(logger)(handler)
	handler = middleware.Recovery(logger)(handler)

	return handler
}
