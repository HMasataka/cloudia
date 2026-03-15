package middleware_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HMasataka/cloudia/internal/gateway/middleware"
)

func newIdempotencyHandler(store *middleware.IdempotencyStore, next http.Handler) http.Handler {
	return middleware.Idempotency(store)(next)
}

func makeRequest(method, path, idempKey, gcpKey, svc string, body []byte) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if idempKey != "" {
		req.Header.Set("X-Amzn-Idempotency-Token", idempKey)
	}
	if gcpKey != "" {
		req.Header.Set("X-Goog-Request-Id", gcpKey)
	}
	if svc != "" {
		req.Header.Set("X-Cloudia-Service", svc)
	}
	return req
}

func TestIdempotency_NoKey_PassThrough(t *testing.T) {
	store := middleware.NewIdempotencyStore(context.Background())
	calls := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	handler := newIdempotencyHandler(store, next)

	// Two requests without idempotency key should both reach next handler
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("body"))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, rec.Code)
		}
	}
	if calls != 2 {
		t.Errorf("expected next to be called 2 times, got %d", calls)
	}
}

func TestIdempotency_SameKeyAndBody_ReturnsCache(t *testing.T) {
	store := middleware.NewIdempotencyStore(context.Background())
	calls := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"abc"}`))
	})

	handler := newIdempotencyHandler(store, next)
	body := []byte(`{"name":"test"}`)

	// First request
	req1 := makeRequest(http.MethodPost, "/", "token-1", "", "s3", body)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("first request: expected 201, got %d", rec1.Code)
	}

	// Second request with same key and body — should get cached response
	req2 := makeRequest(http.MethodPost, "/", "token-1", "", "s3", body)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusCreated {
		t.Errorf("cached request: expected 201, got %d", rec2.Code)
	}
	if rec2.Body.String() != `{"id":"abc"}` {
		t.Errorf("cached request: expected body %q, got %q", `{"id":"abc"}`, rec2.Body.String())
	}
	if calls != 1 {
		t.Errorf("expected next to be called only once, got %d", calls)
	}
}

func TestIdempotency_SameKeyDifferentBody_Returns400(t *testing.T) {
	store := middleware.NewIdempotencyStore(context.Background())
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"abc"}`))
	})

	handler := newIdempotencyHandler(store, next)

	// First request
	req1 := makeRequest(http.MethodPost, "/", "token-2", "", "ec2", []byte(`{"name":"foo"}`))
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("first request: expected 201, got %d", rec1.Code)
	}

	// Second request with same key but different body
	req2 := makeRequest(http.MethodPost, "/", "token-2", "", "ec2", []byte(`{"name":"bar"}`))
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Errorf("mismatch request: expected 400, got %d", rec2.Code)
	}
	if !strings.Contains(rec2.Body.String(), "IdempotentParameterMismatch") {
		t.Errorf("expected IdempotentParameterMismatch in body, got: %q", rec2.Body.String())
	}
}

func TestIdempotency_GCPKey_Works(t *testing.T) {
	store := middleware.NewIdempotencyStore(context.Background())
	calls := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("gcp-response"))
	})

	handler := newIdempotencyHandler(store, next)
	body := []byte(`{"zone":"us-central1-a"}`)

	// First request with GCP key
	req1 := makeRequest(http.MethodPost, "/", "", "gcp-req-id-1", "compute", body)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", rec1.Code)
	}

	// Second request with same GCP key
	req2 := makeRequest(http.MethodPost, "/", "", "gcp-req-id-1", "compute", body)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("cached GCP request: expected 200, got %d", rec2.Code)
	}
	if rec2.Body.String() != "gcp-response" {
		t.Errorf("expected cached body 'gcp-response', got %q", rec2.Body.String())
	}
	if calls != 1 {
		t.Errorf("expected next called once, got %d", calls)
	}
}

func TestIdempotency_DifferentServices_DontCollide(t *testing.T) {
	store := middleware.NewIdempotencyStore(context.Background())
	callCount := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("response"))
	})

	handler := newIdempotencyHandler(store, next)
	body := []byte(`{"data":"value"}`)

	// Same key, different services
	req1 := makeRequest(http.MethodPost, "/", "shared-token", "", "s3", body)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	req2 := makeRequest(http.MethodPost, "/", "shared-token", "", "sqs", body)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if callCount != 2 {
		t.Errorf("expected next called 2 times (different services), got %d", callCount)
	}
	_ = rec1
	_ = rec2
}
