package admin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HMasataka/cloudia/internal/admin"
	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/pkg/models"
	"go.uber.org/zap"
)

func newUIHandler() *admin.Handler {
	logger, _ := zap.NewDevelopment()
	return admin.NewHandler(nil, nil, nil, nil, logger)
}

func newUIHandlerWithStore() *admin.Handler {
	store := newTestStore()
	logger, _ := zap.NewDevelopment()
	return admin.NewHandler(nil, store, nil, nil, logger)
}

func newUIHandlerWithConfig() *admin.Handler {
	logger, _ := zap.NewDevelopment()
	cfg := &config.Config{
		Server: config.ServerConfig{Host: "localhost", Port: 4566},
		Auth: config.AuthConfig{
			Mode: "local",
			AWS: config.AWSAuthConfig{
				AccessKey: "real-key",
				SecretKey: "real-secret",
				AccountID: "123456789",
				Region:    "us-east-1",
			},
		},
	}
	return admin.NewHandler(nil, nil, nil, cfg, logger)
}

func TestDashboardPage(t *testing.T) {
	h := newUIHandler()
	req := httptest.NewRequest(http.MethodGet, "/admin/ui", nil)
	rec := httptest.NewRecorder()
	h.DashboardPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Dashboard") {
		t.Error("expected 'Dashboard' in body")
	}
	if !strings.Contains(body, "Cloudia Admin") {
		t.Error("expected 'Cloudia Admin' in body")
	}
}

func TestDashboardPage_HXRequest(t *testing.T) {
	h := newUIHandler()
	req := httptest.NewRequest(http.MethodGet, "/admin/ui", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	h.DashboardPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	// フラグメントレスポンスなので <html> タグは含まれない
	if strings.Contains(body, "<html") {
		t.Error("HX-Request response should not contain <html> tag")
	}
}

func TestResourcesPage_Empty(t *testing.T) {
	h := newUIHandlerWithStore()
	req := httptest.NewRequest(http.MethodGet, "/admin/ui/resources", nil)
	rec := httptest.NewRecorder()
	h.ResourcesPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Resources") {
		t.Error("expected 'Resources' in body")
	}
}

func TestResourcesPage_WithData(t *testing.T) {
	store := newTestStore()
	ctx := context.Background()
	r := &models.Resource{
		Kind:      "bucket",
		ID:        "test-bucket",
		Provider:  "aws",
		Service:   "s3",
		CreatedAt: time.Now(),
	}
	if err := store.Put(ctx, r); err != nil {
		t.Fatalf("put resource: %v", err)
	}
	logger, _ := zap.NewDevelopment()
	h := admin.NewHandler(nil, store, nil, nil, logger)

	req := httptest.NewRequest(http.MethodGet, "/admin/ui/resources", nil)
	rec := httptest.NewRecorder()
	h.ResourcesPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "test-bucket") {
		t.Error("expected resource ID in body")
	}
}

func TestResourceDetailPage_Found(t *testing.T) {
	store := newTestStore()
	ctx := context.Background()
	r := &models.Resource{
		Kind:      "bucket",
		ID:        "detail-bucket",
		Provider:  "aws",
		Service:   "s3",
		CreatedAt: time.Now(),
	}
	if err := store.Put(ctx, r); err != nil {
		t.Fatalf("put resource: %v", err)
	}
	logger, _ := zap.NewDevelopment()
	h := admin.NewHandler(nil, store, nil, nil, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/ui/resources/{kind}/{id}", h.ResourceDetailPage)

	req := httptest.NewRequest(http.MethodGet, "/admin/ui/resources/bucket/detail-bucket", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "detail-bucket") {
		t.Error("expected resource ID in body")
	}
}

func TestResourceDetailPage_NotFound(t *testing.T) {
	h := newUIHandlerWithStore()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/ui/resources/{kind}/{id}", h.ResourceDetailPage)

	req := httptest.NewRequest(http.MethodGet, "/admin/ui/resources/bucket/nonexistent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestResourceDetailPage_NoStore(t *testing.T) {
	h := newUIHandler()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/ui/resources/{kind}/{id}", h.ResourceDetailPage)

	req := httptest.NewRequest(http.MethodGet, "/admin/ui/resources/bucket/foo", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

func TestContainersPage(t *testing.T) {
	h := newUIHandler()
	req := httptest.NewRequest(http.MethodGet, "/admin/ui/containers", nil)
	rec := httptest.NewRecorder()
	h.ContainersPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Containers") {
		t.Error("expected 'Containers' in body")
	}
}

func TestConfigPage(t *testing.T) {
	h := newUIHandlerWithConfig()
	req := httptest.NewRequest(http.MethodGet, "/admin/ui/config", nil)
	rec := httptest.NewRecorder()
	h.ConfigPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Configuration") {
		t.Error("expected 'Configuration' in body")
	}
	// 機密情報がマスクされているか確認
	if strings.Contains(body, "real-key") {
		t.Error("access key should be masked")
	}
	if strings.Contains(body, "real-secret") {
		t.Error("secret key should be masked")
	}
	if !strings.Contains(body, "***") {
		t.Error("expected masked value '***' in body")
	}
}

func TestConfigPage_NilConfig(t *testing.T) {
	h := newUIHandler()
	req := httptest.NewRequest(http.MethodGet, "/admin/ui/config", nil)
	rec := httptest.NewRecorder()
	h.ConfigPage(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

func TestStaticFS(t *testing.T) {
	h := newUIHandler()
	fs := h.StaticFS()
	if fs == nil {
		t.Error("expected non-nil FileSystem")
	}

	// style.css が含まれているか確認（StaticFS は static/ サブディレクトリを返す）
	f, err := fs.Open("style.css")
	if err != nil {
		t.Errorf("expected style.css to be accessible: %v", err)
	}
	if f != nil {
		f.Close()
	}
}
