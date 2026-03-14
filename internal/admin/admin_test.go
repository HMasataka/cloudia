package admin_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HMasataka/cloudia/internal/admin"
	"go.uber.org/zap"
)

func TestServicesHandler(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	h := admin.NewHandler(logger)

	req := httptest.NewRequest(http.MethodGet, "/admin/services", nil)
	rec := httptest.NewRecorder()

	h.ServicesHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	body := strings.TrimSpace(rec.Body.String())
	expected := `{"services":[]}`
	if body != expected {
		t.Errorf("expected body %s, got %s", expected, body)
	}
}
