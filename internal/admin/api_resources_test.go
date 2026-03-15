package admin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HMasataka/cloudia/internal/admin"
	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
	"go.uber.org/zap"
)

// testStore はテスト用の Store 実装です。
type testStore struct {
	store *state.MemoryStore
}

func newTestStore() *testStore {
	return &testStore{store: state.NewMemoryStore()}
}

func (s *testStore) Get(ctx context.Context, kind, id string) (*models.Resource, error) {
	return s.store.Get(ctx, kind, id)
}

func (s *testStore) List(ctx context.Context, kind string, filter state.Filter) ([]*models.Resource, error) {
	return s.store.List(ctx, kind, filter)
}

func (s *testStore) Put(ctx context.Context, resource *models.Resource) error {
	return s.store.Put(ctx, resource)
}

func (s *testStore) Delete(ctx context.Context, kind, id string) error {
	return s.store.Delete(ctx, kind, id)
}

func (s *testStore) Snapshot(ctx context.Context, path string) error {
	return s.store.Snapshot(ctx, path)
}

func (s *testStore) Restore(ctx context.Context, path string) error {
	return s.store.Restore(ctx, path)
}

// newTestHandler はテスト用の Handler を生成します。
func newTestHandler(store state.Store) *admin.Handler {
	logger, _ := zap.NewDevelopment()
	return admin.NewHandler(nil, store, nil, nil, logger)
}

func newTestHandlerWithAll(store state.Store, reg *service.Registry, cfg *config.Config) *admin.Handler {
	logger, _ := zap.NewDevelopment()
	return admin.NewHandler(nil, store, reg, cfg, logger)
}

func TestListResourcesHandler_Empty(t *testing.T) {
	store := newTestStore()
	h := newTestHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/resources", nil)
	rec := httptest.NewRecorder()

	h.ListResourcesHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if total := resp["total"].(float64); total != 0 {
		t.Errorf("expected total=0, got %v", total)
	}
	if data := resp["data"].([]any); len(data) != 0 {
		t.Errorf("expected empty data, got %v", data)
	}
}

func TestListResourcesHandler_Pagination(t *testing.T) {
	store := newTestStore()
	ctx := context.Background()

	// 5件登録
	for i := 0; i < 5; i++ {
		r := &models.Resource{
			Kind:      "bucket",
			ID:        "bucket-" + string(rune('a'+i)),
			Provider:  "aws",
			Service:   "s3",
			CreatedAt: time.Now(),
		}
		if err := store.Put(ctx, r); err != nil {
			t.Fatalf("put resource: %v", err)
		}
	}

	h := newTestHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/resources?page=1&per_page=2", nil)
	rec := httptest.NewRecorder()
	h.ListResourcesHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if total := resp["total"].(float64); total != 5 {
		t.Errorf("expected total=5, got %v", total)
	}
	if data := resp["data"].([]any); len(data) != 2 {
		t.Errorf("expected 2 items on page 1, got %d", len(data))
	}
	if perPage := resp["per_page"].(float64); perPage != 2 {
		t.Errorf("expected per_page=2, got %v", perPage)
	}
}

func TestListResourcesHandler_MaxPerPage(t *testing.T) {
	store := newTestStore()
	h := newTestHandler(store)

	// per_page=300 を指定 → 200 に制限される
	req := httptest.NewRequest(http.MethodGet, "/admin/api/resources?per_page=300", nil)
	rec := httptest.NewRecorder()
	h.ListResourcesHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if perPage := resp["per_page"].(float64); perPage != 200 {
		t.Errorf("expected per_page capped at 200, got %v", perPage)
	}
}

func TestGetResourceHandler_Found(t *testing.T) {
	store := newTestStore()
	ctx := context.Background()

	r := &models.Resource{Kind: "bucket", ID: "my-bucket", Provider: "aws"}
	if err := store.Put(ctx, r); err != nil {
		t.Fatalf("put resource: %v", err)
	}

	h := newTestHandler(store)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/api/resources/{kind}/{id}", h.GetResourceHandler)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/resources/bucket/my-bucket", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var got models.Resource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "my-bucket" {
		t.Errorf("expected ID=my-bucket, got %q", got.ID)
	}
}

func TestGetResourceHandler_NotFound(t *testing.T) {
	store := newTestStore()
	h := newTestHandler(store)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/api/resources/{kind}/{id}", h.GetResourceHandler)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/resources/bucket/nonexistent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] == "" {
		t.Error("expected error message in response")
	}
}

func TestDeleteResourceHandler_Found(t *testing.T) {
	store := newTestStore()
	ctx := context.Background()

	r := &models.Resource{Kind: "bucket", ID: "del-bucket", Provider: "aws"}
	if err := store.Put(ctx, r); err != nil {
		t.Fatalf("put resource: %v", err)
	}

	h := newTestHandler(store)

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /admin/api/resources/{kind}/{id}", h.DeleteResourceHandler)

	req := httptest.NewRequest(http.MethodDelete, "/admin/api/resources/bucket/del-bucket", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", rec.Code)
	}

	// 削除後は Get で NotFound になるはず
	_, err := store.Get(ctx, "bucket", "del-bucket")
	if err == nil {
		t.Error("expected resource to be deleted but Get succeeded")
	}
}

func TestDeleteResourceHandler_NotFound(t *testing.T) {
	store := newTestStore()
	h := newTestHandler(store)

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /admin/api/resources/{kind}/{id}", h.DeleteResourceHandler)

	req := httptest.NewRequest(http.MethodDelete, "/admin/api/resources/bucket/nonexistent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

func TestListServicesHandler_NoRegistry(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	h := admin.NewHandler(nil, nil, nil, nil, logger)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/services", nil)
	rec := httptest.NewRecorder()
	h.ListServicesHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	data := resp["data"].([]any)
	if len(data) != 0 {
		t.Errorf("expected empty data, got %d items", len(data))
	}
}

func TestGetConfigHandler_Masked(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			Mode: "local",
			AWS: config.AWSAuthConfig{
				AccessKey: "real-access-key",
				SecretKey: "real-secret-key",
				AccountID: "123456789",
				Region:    "us-east-1",
			},
			GCP: config.GCPAuthConfig{
				CredentialsFile: "/path/to/credentials.json",
				Project:         "my-project",
				Zone:            "us-central1-a",
			},
		},
	}

	h := newTestHandlerWithAll(nil, nil, cfg)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/config", nil)
	rec := httptest.NewRecorder()
	h.GetConfigHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	auth := resp["auth"].(map[string]any)
	aws := auth["aws"].(map[string]any)
	gcp := auth["gcp"].(map[string]any)

	if aws["access_key"] != "***" {
		t.Errorf("expected access_key masked, got %v", aws["access_key"])
	}
	if aws["secret_key"] != "***" {
		t.Errorf("expected secret_key masked, got %v", aws["secret_key"])
	}
	if gcp["credentials_file"] != "***" {
		t.Errorf("expected credentials_file masked, got %v", gcp["credentials_file"])
	}

	// マスクされないフィールドは元の値
	if aws["account_id"] != "123456789" {
		t.Errorf("expected account_id=123456789, got %v", aws["account_id"])
	}
	if aws["region"] != "us-east-1" {
		t.Errorf("expected region=us-east-1, got %v", aws["region"])
	}
	if gcp["project"] != "my-project" {
		t.Errorf("expected project=my-project, got %v", gcp["project"])
	}
}

func TestGetConfigHandler_NilConfig(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	h := admin.NewHandler(nil, nil, nil, nil, logger)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/config", nil)
	rec := httptest.NewRecorder()
	h.GetConfigHandler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rec.Code)
	}
}

func TestListContainersHandler_NoDockerClient(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	h := admin.NewHandler(nil, nil, nil, nil, logger)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/containers", nil)
	rec := httptest.NewRecorder()
	h.ListContainersHandler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rec.Code)
	}
}

func TestContainerLogsHandler_NoDockerClient(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	h := admin.NewHandler(nil, nil, nil, nil, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/api/containers/{id}/logs", h.ContainerLogsHandler)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/containers/abc123/logs", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rec.Code)
	}
}
