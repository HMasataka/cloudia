package admin_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HMasataka/cloudia/internal/admin"
	"go.uber.org/zap"
)

func TestHealthHandler(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	h := admin.NewHandler(nil, nil, nil, nil, logger)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	h.HealthHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	body := strings.TrimSpace(rec.Body.String())
	expected := `{"status":"ok"}`
	if body != expected {
		t.Errorf("expected body %s, got %s", expected, body)
	}
}
