package middleware

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// metricsContextKey はメトリクス情報をコンテキストに格納するためのキー型です。
type metricsContextKey string

const metricsInfoKey metricsContextKey = "metrics_info"

// MetricsInfo はリクエストのプロバイダ/サービス/アクションラベルを保持するミュータブルな構造体です。
// ポインタでコンテキストに格納することで、ハンドラ内から書き込み、ミドルウェアから読み取ることができます。
type MetricsInfo struct {
	Provider string
	Service  string
	Action   string
}

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cloudia_http_requests_total",
			Help: "Total number of HTTP requests processed by Cloudia.",
		},
		[]string{"provider", "service", "action", "status"},
	)

	httpRequestDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cloudia_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"provider", "service", "action", "status"},
	)

	httpErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cloudia_http_errors_total",
			Help: "Total number of HTTP error responses (4xx and 5xx) from Cloudia.",
		},
		[]string{"provider", "service", "action", "status"},
	)
)

// MetricsInfoFromContext はコンテキストから MetricsInfo を取得します。
// ミドルウェアが事前にコンテキストに設定した場合にのみ有効です。
func MetricsInfoFromContext(ctx context.Context) *MetricsInfo {
	v, _ := ctx.Value(metricsInfoKey).(*MetricsInfo)
	return v
}

// metricsResponseWriter は http.ResponseWriter をラップしてステータスコードをキャプチャします。
type metricsResponseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *metricsResponseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// Metrics はPrometheusメトリクスを記録するミドルウェアを返します。
// プロバイダ/サービス/アクションラベルはリクエストコンテキストに格納した MetricsInfo から取得します。
// ServiceHandler は MetricsInfoFromContext でポインタを取得し、フィールドを直接書き込んでください。
func Metrics() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			info := &MetricsInfo{
				Provider: "unknown",
				Service:  "unknown",
				Action:   "unknown",
			}
			ctx := context.WithValue(r.Context(), metricsInfoKey, info)
			r = r.WithContext(ctx)

			start := time.Now()
			rw := &metricsResponseWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rw, r)

			duration := time.Since(start).Seconds()
			status := strconv.Itoa(rw.status)

			labels := prometheus.Labels{
				"provider": info.Provider,
				"service":  info.Service,
				"action":   info.Action,
				"status":   status,
			}

			httpRequestsTotal.With(labels).Inc()
			httpRequestDurationSeconds.With(labels).Observe(duration)

			if rw.status >= 400 {
				httpErrorsTotal.With(labels).Inc()
			}
		})
	}
}
