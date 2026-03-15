package gateway

import (
	"context"
	"net/http"
	"time"

	"github.com/HMasataka/cloudia/internal/admin"
	"github.com/HMasataka/cloudia/internal/gateway/middleware"
	"go.uber.org/zap"
)

// NewRouter は HTTP ルーターを構築して返します。
// ミドルウェアチェイン: recovery → logging → idempotency → timeout → metrics → mux の順にラップします。
// ctx のキャンセルで idempotency GC goroutine が停止します。
func NewRouter(ctx context.Context, adminHandler *admin.Handler, serviceHandler *ServiceHandler, logger *zap.Logger, timeout time.Duration) http.Handler {
	return NewRouterWithIdempotency(adminHandler, serviceHandler, logger, timeout, middleware.NewIdempotencyStore(ctx))
}

// NewRouterWithIdempotency は冪等性ストアを受け取って HTTP ルーターを構築して返します。
func NewRouterWithIdempotency(adminHandler *admin.Handler, serviceHandler *ServiceHandler, logger *zap.Logger, timeout time.Duration, idempStore *middleware.IdempotencyStore) http.Handler {
	mux := http.NewServeMux()

	// 既存ルート
	mux.HandleFunc("GET /health", adminHandler.HealthHandler)
	mux.HandleFunc("GET /admin/services", adminHandler.ServicesHandler)

	// Admin API ルート
	mux.HandleFunc("GET /admin/api/resources", adminHandler.ListResourcesHandler)
	mux.HandleFunc("GET /admin/api/resources/{kind}/{id}", adminHandler.GetResourceHandler)
	mux.HandleFunc("DELETE /admin/api/resources/{kind}/{id}", adminHandler.DeleteResourceHandler)
	mux.HandleFunc("GET /admin/api/services", adminHandler.ListServicesHandler)
	mux.HandleFunc("GET /admin/api/containers", adminHandler.ListContainersHandler)
	mux.HandleFunc("GET /admin/api/containers/{id}/logs", adminHandler.ContainerLogsHandler)
	mux.HandleFunc("GET /admin/api/config", adminHandler.GetConfigHandler)

	// Admin UI ルート
	mux.HandleFunc("GET /admin/ui", adminHandler.DashboardPage)
	mux.HandleFunc("GET /admin/ui/resources", adminHandler.ResourcesPage)
	mux.HandleFunc("GET /admin/ui/resources/{kind}/{id}", adminHandler.ResourceDetailPage)
	mux.HandleFunc("GET /admin/ui/containers", adminHandler.ContainersPage)
	mux.HandleFunc("GET /admin/ui/config", adminHandler.ConfigPage)

	// 静的アセット配信
	mux.Handle("/admin/static/", http.StripPrefix("/admin/static/", http.FileServer(adminHandler.StaticFS())))

	mux.Handle("/", serviceHandler)

	var handler http.Handler = mux
	handler = middleware.Timeout(timeout)(handler)
	handler = middleware.Metrics()(handler)
	handler = middleware.Idempotency(idempStore)(handler)
	handler = middleware.Logging(logger)(handler)
	handler = middleware.Recovery(logger)(handler)

	return handler
}
