package gateway_test

import (
	"context"
	"testing"

	"github.com/HMasataka/cloudia/internal/gateway"
)

func TestGenerateRequestID_Format(t *testing.T) {
	// Given / When: generate a request ID
	id, err := gateway.GenerateRequestID()

	// Then: no error and UUID v4 format
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(id) != 36 {
		t.Errorf("expected UUID length 36, got %d: %q", len(id), id)
	}
	// verify version nibble is '4'
	if id[14] != '4' {
		t.Errorf("expected version nibble '4', got %q", id[14])
	}
	// verify variant bits: position 19 must be 8, 9, a, or b
	v := id[19]
	if v != '8' && v != '9' && v != 'a' && v != 'b' {
		t.Errorf("expected variant character 8/9/a/b, got %q", v)
	}
}

func TestGenerateRequestID_Unique(t *testing.T) {
	// Given: generate two IDs
	id1, err1 := gateway.GenerateRequestID()
	id2, err2 := gateway.GenerateRequestID()

	// Then: both succeed and differ
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v, %v", err1, err2)
	}
	if id1 == id2 {
		t.Errorf("expected unique IDs, got identical: %q", id1)
	}
}

func TestWithRequestID_RoundTrip(t *testing.T) {
	// Given: a context with a request ID
	ctx := context.Background()
	want := "test-request-id"

	// When: stored and retrieved
	ctx = gateway.WithRequestID(ctx, want)
	got := gateway.RequestIDFromContext(ctx)

	// Then: retrieved value matches
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestRequestIDFromContext_Missing(t *testing.T) {
	// Given: a context without a request ID
	ctx := context.Background()

	// When: retrieved
	got := gateway.RequestIDFromContext(ctx)

	// Then: empty string returned
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}
