package s3_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HMasataka/cloudia/internal/config"
	s3svc "github.com/HMasataka/cloudia/internal/service/s3"
	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
)

// parsePath is tested indirectly through ServeHTTP observable behavior:
// - path with key component (e.g. /bucket/key) must NOT trigger state updates
// - root path "/" must NOT trigger state updates
// - bucket-only path (e.g. /bucket) triggers state updates on matching method+status

func TestServeHTTP_CreateBucket_200_StoresResource(t *testing.T) {
	// Given: a backend returning 200 for PUT /my-bucket
	// When: ServeHTTP is called
	// Then: a bucket resource is stored in the State Store

	store := state.NewMemoryStore()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	svc := s3svc.NewS3ServiceWithEndpoint(config.AWSAuthConfig{}, backend.URL, store)

	req := httptest.NewRequest(http.MethodPut, "/my-bucket", nil)
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ServeHTTP status = %d, want %d", rec.Code, http.StatusOK)
	}

	resource, err := store.Get(context.Background(), "aws:s3:bucket", "my-bucket")
	if err != nil {
		t.Fatalf("store.Get() error = %v, want nil", err)
	}
	if resource.ID != "my-bucket" {
		t.Errorf("resource.ID = %q, want %q", resource.ID, "my-bucket")
	}
	if resource.Kind != "aws:s3:bucket" {
		t.Errorf("resource.Kind = %q, want %q", resource.Kind, "aws:s3:bucket")
	}
	if resource.Provider != "aws" {
		t.Errorf("resource.Provider = %q, want %q", resource.Provider, "aws")
	}
	if resource.Service != "s3" {
		t.Errorf("resource.Service = %q, want %q", resource.Service, "s3")
	}
}

func TestServeHTTP_DeleteBucket_204_RemovesResource(t *testing.T) {
	// Given: a store with an existing bucket resource and a backend returning 204
	// When: ServeHTTP is called with DELETE /my-bucket
	// Then: the bucket resource is removed from the State Store

	store := state.NewMemoryStore()
	if err := store.Put(context.Background(), &models.Resource{
		Kind:     "aws:s3:bucket",
		ID:       "my-bucket",
		Provider: "aws",
		Service:  "s3",
	}); err != nil {
		t.Fatalf("store.Put() error = %v", err)
	}

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()

	svc := s3svc.NewS3ServiceWithEndpoint(config.AWSAuthConfig{}, backend.URL, store)

	req := httptest.NewRequest(http.MethodDelete, "/my-bucket", nil)
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("ServeHTTP status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	_, err := store.Get(context.Background(), "aws:s3:bucket", "my-bucket")
	if err == nil {
		t.Error("store.Get() should return error after DeleteBucket, but got nil")
	}
}

func TestServeHTTP_PutObject_DoesNotUpdateStore(t *testing.T) {
	// Given: a backend returning 200 for PUT /my-bucket/my-key (object upload)
	// When: ServeHTTP is called
	// Then: no resource is written to the State Store (key is non-empty, not a bucket op)

	store := state.NewMemoryStore()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	svc := s3svc.NewS3ServiceWithEndpoint(config.AWSAuthConfig{}, backend.URL, store)

	req := httptest.NewRequest(http.MethodPut, "/my-bucket/my-key", nil)
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	resources, err := store.List(context.Background(), "aws:s3:bucket", state.Filter{})
	if err != nil {
		t.Fatalf("store.List() error = %v", err)
	}
	if len(resources) != 0 {
		t.Errorf("store.List() len = %d, want 0 (object PUT must not create bucket resource)", len(resources))
	}
}

func TestServeHTTP_MinioUnavailable_ReturnsS3XmlError(t *testing.T) {
	// Given: an endpoint that is not listening
	// When: ServeHTTP is called
	// Then: an S3-compatible XML error response with 502 status is returned

	store := state.NewMemoryStore()
	svc := s3svc.NewS3ServiceWithEndpoint(config.AWSAuthConfig{}, "http://localhost:19998", store)

	req := httptest.NewRequest(http.MethodGet, "/my-bucket", nil)
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("ServeHTTP status = %d, want %d", rec.Code, http.StatusBadGateway)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/xml" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/xml")
	}
}

func TestServeHTTP_CreateBucket_Non200_DoesNotUpdateStore(t *testing.T) {
	// Given: a backend returning 409 for PUT /my-bucket (bucket already exists)
	// When: ServeHTTP is called
	// Then: no resource is written to the State Store

	store := state.NewMemoryStore()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer backend.Close()

	svc := s3svc.NewS3ServiceWithEndpoint(config.AWSAuthConfig{}, backend.URL, store)

	req := httptest.NewRequest(http.MethodPut, "/my-bucket", nil)
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	resources, err := store.List(context.Background(), "aws:s3:bucket", state.Filter{})
	if err != nil {
		t.Fatalf("store.List() error = %v", err)
	}
	if len(resources) != 0 {
		t.Errorf("store.List() len = %d, want 0 (failed CreateBucket must not store resource)", len(resources))
	}
}

func TestServeHTTP_RootPath_DoesNotUpdateStore(t *testing.T) {
	// Given: a backend returning 200 for GET /
	// When: ServeHTTP is called with root path
	// Then: no resource is written to the State Store (no bucket in path)

	store := state.NewMemoryStore()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	svc := s3svc.NewS3ServiceWithEndpoint(config.AWSAuthConfig{}, backend.URL, store)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	resources, err := store.List(context.Background(), "aws:s3:bucket", state.Filter{})
	if err != nil {
		t.Fatalf("store.List() error = %v", err)
	}
	if len(resources) != 0 {
		t.Errorf("store.List() len = %d, want 0 (root path must not affect store)", len(resources))
	}
}
