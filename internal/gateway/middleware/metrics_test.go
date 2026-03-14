package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/HMasataka/cloudia/internal/gateway/middleware"
)

func TestMetrics_RecordsRequestsTotal(t *testing.T) {
	// 各テストで独立したレジストリを使うため、デフォルトレジストリはグローバルなので
	// ここでは testutil.ToFloat64 で promauto 登録済みメトリクスを確認する。
	// ただし promauto はデフォルトレジストリを使うため、複数テストで累積される点に注意。

	handler := middleware.Metrics()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := middleware.MetricsInfoFromContext(r.Context())
		if info != nil {
			info.Provider = "aws"
			info.Service = "s3"
			info.Action = "ListBuckets"
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	// cloudia_http_requests_total{action="ListBuckets",provider="aws",service="s3",status="200"} が 1 以上あること
	counter, err := prometheus.DefaultRegisterer.(prometheus.Gatherer).Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}

	found := false
	for _, mf := range counter {
		if mf.GetName() == "cloudia_http_requests_total" {
			for _, m := range mf.GetMetric() {
				labels := map[string]string{}
				for _, lp := range m.GetLabel() {
					labels[lp.GetName()] = lp.GetValue()
				}
				if labels["provider"] == "aws" && labels["service"] == "s3" && labels["action"] == "ListBuckets" && labels["status"] == "200" {
					if m.GetCounter().GetValue() >= 1 {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Error("expected cloudia_http_requests_total with aws/s3/ListBuckets/200 to be >= 1")
	}
}

func TestMetrics_RecordsErrors(t *testing.T) {
	handler := middleware.Metrics()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := middleware.MetricsInfoFromContext(r.Context())
		if info != nil {
			info.Provider = "aws"
			info.Service = "s3"
			info.Action = "PutBucket"
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))

	req := httptest.NewRequest(http.MethodPut, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}

	// cloudia_http_errors_total が記録されていること
	err := testutil.GatherAndCompare(
		prometheus.DefaultGatherer,
		strings.NewReader(`
# HELP cloudia_http_errors_total Total number of HTTP error responses (4xx and 5xx) from Cloudia.
# TYPE cloudia_http_errors_total counter
`),
		// メトリクス名だけ確認（値の詳細は累積されるため省略）
	)
	// GatherAndCompare の部分一致ではなく存在確認のみ行う
	_ = err

	gathered, _ := prometheus.DefaultGatherer.Gather()
	found := false
	for _, mf := range gathered {
		if mf.GetName() == "cloudia_http_errors_total" {
			for _, m := range mf.GetMetric() {
				labels := map[string]string{}
				for _, lp := range m.GetLabel() {
					labels[lp.GetName()] = lp.GetValue()
				}
				if labels["provider"] == "aws" && labels["service"] == "s3" && labels["action"] == "PutBucket" && labels["status"] == "500" {
					if m.GetCounter().GetValue() >= 1 {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Error("expected cloudia_http_errors_total with aws/s3/PutBucket/500 to be >= 1")
	}
}

func TestMetrics_UnknownLabelsWhenHandlerDoesNotSetInfo(t *testing.T) {
	handler := middleware.Metrics()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// MetricsInfo を設定しない
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	// unknown ラベルで記録されていること
	gathered, _ := prometheus.DefaultGatherer.Gather()
	found := false
	for _, mf := range gathered {
		if mf.GetName() == "cloudia_http_requests_total" {
			for _, m := range mf.GetMetric() {
				labels := map[string]string{}
				for _, lp := range m.GetLabel() {
					labels[lp.GetName()] = lp.GetValue()
				}
				if labels["provider"] == "unknown" && labels["service"] == "unknown" && labels["action"] == "unknown" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("expected cloudia_http_requests_total with unknown labels")
	}
}
