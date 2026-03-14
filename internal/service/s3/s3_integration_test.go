package s3_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HMasataka/cloudia/internal/config"
	s3svc "github.com/HMasataka/cloudia/internal/service/s3"
	"github.com/HMasataka/cloudia/internal/state"
)

// minioMux builds an http.Handler that mimics the MinIO S3 API for CRUD operations.
// It records received requests and returns realistic status codes and bodies.
func minioMux(t *testing.T) http.Handler {
	t.Helper()

	mux := http.NewServeMux()

	// ListBuckets: GET /
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+ //nolint:errcheck
			`<ListAllMyBucketsResult><Owner><ID>test</ID><DisplayName>test</DisplayName></Owner>`+
			`<Buckets></Buckets></ListAllMyBucketsResult>`)
	})

	// CreateBucket: PUT /test-bucket
	mux.HandleFunc("PUT /test-bucket", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/test-bucket" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// PutObject: PUT /test-bucket/test.txt
	mux.HandleFunc("PUT /test-bucket/test.txt", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil || len(body) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// GetObject: GET /test-bucket/test.txt
	mux.HandleFunc("GET /test-bucket/test.txt", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "hello world") //nolint:errcheck
	})

	// DeleteObject: DELETE /test-bucket/test.txt
	mux.HandleFunc("DELETE /test-bucket/test.txt", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// DeleteBucket: DELETE /test-bucket
	mux.HandleFunc("DELETE /test-bucket", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	return mux
}

func newIntegrationSvc(t *testing.T, backendURL string) *s3svc.S3Service {
	t.Helper()
	store := state.NewMemoryStore()
	return s3svc.NewS3ServiceWithEndpointAndStore(config.AWSAuthConfig{}, backendURL, store)
}

func TestIntegration_CreateBucket(t *testing.T) {
	// Given: a MinIO mock that accepts PUT /test-bucket with 200
	// When: ServeHTTP is called with PUT /test-bucket
	// Then: response status is 200
	backend := httptest.NewServer(minioMux(t))
	defer backend.Close()

	svc := newIntegrationSvc(t, backend.URL)

	req := httptest.NewRequest(http.MethodPut, "/test-bucket", nil)
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("CreateBucket status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestIntegration_ListBuckets(t *testing.T) {
	// Given: a MinIO mock that returns XML for GET /
	// When: ServeHTTP is called with GET /
	// Then: response status is 200 and body contains XML
	backend := httptest.NewServer(minioMux(t))
	defer backend.Close()

	svc := newIntegrationSvc(t, backend.URL)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ListBuckets status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "xml") {
		t.Errorf("ListBuckets body = %q, want XML content", rec.Body.String())
	}
}

func TestIntegration_PutObject(t *testing.T) {
	// Given: a MinIO mock that accepts PUT /test-bucket/test.txt with a body
	// When: ServeHTTP is called with PUT /test-bucket/test.txt and a non-empty body
	// Then: response status is 200
	backend := httptest.NewServer(minioMux(t))
	defer backend.Close()

	svc := newIntegrationSvc(t, backend.URL)

	req := httptest.NewRequest(http.MethodPut, "/test-bucket/test.txt", strings.NewReader("hello world"))
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("PutObject status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestIntegration_GetObject(t *testing.T) {
	// Given: a MinIO mock that returns a body for GET /test-bucket/test.txt
	// When: ServeHTTP is called with GET /test-bucket/test.txt
	// Then: response status is 200 and body matches the stored content
	backend := httptest.NewServer(minioMux(t))
	defer backend.Close()

	svc := newIntegrationSvc(t, backend.URL)

	req := httptest.NewRequest(http.MethodGet, "/test-bucket/test.txt", nil)
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GetObject status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "hello world" {
		t.Errorf("GetObject body = %q, want %q", body, "hello world")
	}
}

func TestIntegration_DeleteObject(t *testing.T) {
	// Given: a MinIO mock that accepts DELETE /test-bucket/test.txt with 204
	// When: ServeHTTP is called with DELETE /test-bucket/test.txt
	// Then: response status is 204
	backend := httptest.NewServer(minioMux(t))
	defer backend.Close()

	svc := newIntegrationSvc(t, backend.URL)

	req := httptest.NewRequest(http.MethodDelete, "/test-bucket/test.txt", nil)
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("DeleteObject status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestIntegration_DeleteBucket(t *testing.T) {
	// Given: a MinIO mock that accepts DELETE /test-bucket with 204
	// When: ServeHTTP is called with DELETE /test-bucket
	// Then: response status is 204
	backend := httptest.NewServer(minioMux(t))
	defer backend.Close()

	svc := newIntegrationSvc(t, backend.URL)

	req := httptest.NewRequest(http.MethodDelete, "/test-bucket", nil)
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("DeleteBucket status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}
