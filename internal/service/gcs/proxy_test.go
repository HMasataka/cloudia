package gcs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HMasataka/cloudia/internal/config"
	gcssvc "github.com/HMasataka/cloudia/internal/service/gcs"
	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
)

// minioListBucketsXML is a minimal S3 ListAllMyBucketsResult XML used as a mock MinIO response.
const minioListBucketsXML = `<?xml version="1.0" encoding="UTF-8"?>
<ListAllMyBucketsResult>
  <Buckets>
    <Bucket>
      <Name>test-bucket</Name>
      <CreationDate>2024-01-01T00:00:00Z</CreationDate>
    </Bucket>
  </Buckets>
</ListAllMyBucketsResult>`

func TestServeHTTP_CreateBucket(t *testing.T) {
	// Given: a MinIO mock returning 200 for PUT /my-bucket
	// When: POST /storage/v1/b with {"name":"my-bucket"}
	// Then: 200 + GCS JSON bucket response and resource stored in State Store

	store := state.NewMemoryStore()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/my-bucket" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer backend.Close()

	svc := gcssvc.NewGCSServiceWithEndpoint(config.AWSAuthConfig{}, backend.URL, store)

	body := `{"name":"my-bucket"}`
	req := httptest.NewRequest(http.MethodPost, "/storage/v1/b", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ServeHTTP status = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if resp["kind"] != "storage#bucket" {
		t.Errorf("kind = %q, want %q", resp["kind"], "storage#bucket")
	}
	if resp["name"] != "my-bucket" {
		t.Errorf("name = %q, want %q", resp["name"], "my-bucket")
	}

	resource, err := store.Get(context.Background(), "gcp:storage:bucket", "my-bucket")
	if err != nil {
		t.Fatalf("store.Get() error = %v, want nil", err)
	}
	if resource.ID != "my-bucket" {
		t.Errorf("resource.ID = %q, want %q", resource.ID, "my-bucket")
	}
	if resource.Kind != "gcp:storage:bucket" {
		t.Errorf("resource.Kind = %q, want %q", resource.Kind, "gcp:storage:bucket")
	}
	if resource.Provider != "gcp" {
		t.Errorf("resource.Provider = %q, want %q", resource.Provider, "gcp")
	}
	if resource.Service != "storage" {
		t.Errorf("resource.Service = %q, want %q", resource.Service, "storage")
	}
}

func TestServeHTTP_ListBuckets(t *testing.T) {
	// Given: a MinIO mock returning S3 XML for GET /
	// When: GET /storage/v1/b
	// Then: 200 + GCS JSON bucket list

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/" {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, minioListBucketsXML) //nolint:errcheck
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer backend.Close()

	svc := gcssvc.NewGCSServiceWithEndpoint(config.AWSAuthConfig{}, backend.URL, nil)

	req := httptest.NewRequest(http.MethodGet, "/storage/v1/b", nil)
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ServeHTTP status = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if resp["kind"] != "storage#buckets" {
		t.Errorf("kind = %q, want %q", resp["kind"], "storage#buckets")
	}

	items, ok := resp["items"].([]interface{})
	if !ok {
		t.Fatal("items is not an array")
	}
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}

	bucket, ok := items[0].(map[string]interface{})
	if !ok {
		t.Fatal("item[0] is not an object")
	}
	if bucket["name"] != "test-bucket" {
		t.Errorf("bucket name = %q, want %q", bucket["name"], "test-bucket")
	}
}

func TestServeHTTP_GetBucket(t *testing.T) {
	// Given: a MinIO mock returning 200 for HEAD /my-bucket
	// When: GET /storage/v1/b/my-bucket
	// Then: 200 + GCS JSON bucket info

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && r.URL.Path == "/my-bucket" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer backend.Close()

	svc := gcssvc.NewGCSServiceWithEndpoint(config.AWSAuthConfig{}, backend.URL, nil)

	req := httptest.NewRequest(http.MethodGet, "/storage/v1/b/my-bucket", nil)
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ServeHTTP status = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if resp["kind"] != "storage#bucket" {
		t.Errorf("kind = %q, want %q", resp["kind"], "storage#bucket")
	}
	if resp["name"] != "my-bucket" {
		t.Errorf("name = %q, want %q", resp["name"], "my-bucket")
	}
}

func TestServeHTTP_DeleteBucket(t *testing.T) {
	// Given: a store with an existing bucket and a MinIO mock returning 204
	// When: DELETE /storage/v1/b/my-bucket
	// Then: 204 and resource removed from State Store

	store := state.NewMemoryStore()
	if err := store.Put(context.Background(), &models.Resource{
		Kind:     "gcp:storage:bucket",
		ID:       "my-bucket",
		Provider: "gcp",
		Service:  "storage",
	}); err != nil {
		t.Fatalf("store.Put() error = %v", err)
	}

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/my-bucket" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer backend.Close()

	svc := gcssvc.NewGCSServiceWithEndpoint(config.AWSAuthConfig{}, backend.URL, store)

	req := httptest.NewRequest(http.MethodDelete, "/storage/v1/b/my-bucket", nil)
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("ServeHTTP status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	_, err := store.Get(context.Background(), "gcp:storage:bucket", "my-bucket")
	if err == nil {
		t.Error("store.Get() should return error after DeleteBucket, but got nil")
	}
}

func TestServeHTTP_UploadObject_SimpleUpload(t *testing.T) {
	// Given: a MinIO mock returning 200 for PUT /my-bucket/my-key
	// When: PUT /storage/v1/b/my-bucket/o/my-key with uploadType=media
	// Then: 200 proxied from MinIO

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/my-bucket/my-key" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer backend.Close()

	svc := gcssvc.NewGCSServiceWithEndpoint(config.AWSAuthConfig{}, backend.URL, nil)

	body := bytes.NewBufferString("hello world")
	req := httptest.NewRequest(http.MethodPut, "/upload/storage/v1/b/my-bucket/o/my-key?uploadType=media", body)
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ServeHTTP status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestServeHTTP_GetObjectMedia(t *testing.T) {
	// Given: a MinIO mock streaming object content for GET /my-bucket/my-key
	// When: GET /storage/v1/b/my-bucket/o/my-key?alt=media
	// Then: 200 with streamed content

	const objectContent = "object content data"

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/my-bucket/my-key" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, objectContent) //nolint:errcheck
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer backend.Close()

	svc := gcssvc.NewGCSServiceWithEndpoint(config.AWSAuthConfig{}, backend.URL, nil)

	req := httptest.NewRequest(http.MethodGet, "/storage/v1/b/my-bucket/o/my-key?alt=media", nil)
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ServeHTTP status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != objectContent {
		t.Errorf("body = %q, want %q", body, objectContent)
	}
}

func TestServeHTTP_ListObjects(t *testing.T) {
	// Given: a MinIO mock returning 200 for GET /my-bucket
	// When: GET /storage/v1/b/my-bucket/o
	// Then: 200 proxied from MinIO

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/my-bucket" {
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, `<?xml version="1.0"?><ListBucketResult></ListBucketResult>`) //nolint:errcheck
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer backend.Close()

	svc := gcssvc.NewGCSServiceWithEndpoint(config.AWSAuthConfig{}, backend.URL, nil)

	req := httptest.NewRequest(http.MethodGet, "/storage/v1/b/my-bucket/o", nil)
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ServeHTTP status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestServeHTTP_ResumableUpload_Returns501(t *testing.T) {
	// Given: a GCS service
	// When: POST /upload/storage/v1/b/my-bucket/o?uploadType=resumable
	// Then: 501 Not Implemented with GCS JSON error

	svc := gcssvc.NewGCSServiceWithEndpoint(config.AWSAuthConfig{}, "http://localhost:19998", nil)

	req := httptest.NewRequest(http.MethodPost, "/upload/storage/v1/b/my-bucket/o?uploadType=resumable", nil)
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("ServeHTTP status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error response body: %v", err)
	}
	if _, ok := resp["error"]; !ok {
		t.Error("response body should contain an \"error\" field")
	}
}
