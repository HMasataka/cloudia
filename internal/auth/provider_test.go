package auth_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/HMasataka/cloudia/internal/auth"
)

func TestDetectProvider_AWSAuthorizationHeader(t *testing.T) {
	r, _ := http.NewRequest(http.MethodGet, "https://s3.amazonaws.com/bucket/key", nil)
	r.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=AKID/20260315/us-east-1/s3/aws4_request, SignedHeaders=host, Signature=abc123")

	got, err := auth.DetectProvider(r)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "aws" {
		t.Fatalf("expected aws, got %s", got)
	}
}

func TestDetectProvider_XAmzTargetHeader(t *testing.T) {
	r, _ := http.NewRequest(http.MethodPost, "https://dynamodb.us-east-1.amazonaws.com/", nil)
	r.Header.Set("X-Amz-Target", "DynamoDB_20120810.GetItem")

	got, err := auth.DetectProvider(r)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "aws" {
		t.Fatalf("expected aws, got %s", got)
	}
}

func TestDetectProvider_XAmzDateHeader(t *testing.T) {
	r, _ := http.NewRequest(http.MethodGet, "https://s3.amazonaws.com/bucket/key", nil)
	r.Header.Set("X-Amz-Date", "20260315T000000Z")

	got, err := auth.DetectProvider(r)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "aws" {
		t.Fatalf("expected aws, got %s", got)
	}
}

func TestDetectProvider_GCPBearerToken(t *testing.T) {
	r, _ := http.NewRequest(http.MethodGet, "https://storage.googleapis.com/storage/v1/b/bucket", nil)
	r.Header.Set("Authorization", "Bearer ya29.token")

	got, err := auth.DetectProvider(r)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "gcp" {
		t.Fatalf("expected gcp, got %s", got)
	}
}

func TestDetectProvider_GCPStoragePath(t *testing.T) {
	r, _ := http.NewRequest(http.MethodGet, "https://storage.googleapis.com/storage/v1/b/bucket/o/obj", nil)

	got, err := auth.DetectProvider(r)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "gcp" {
		t.Fatalf("expected gcp, got %s", got)
	}
}

func TestDetectProvider_GCPComputePath(t *testing.T) {
	r, _ := http.NewRequest(http.MethodGet, "https://compute.googleapis.com/compute/v1/projects/myproj/zones/us-central1-a/instances", nil)

	got, err := auth.DetectProvider(r)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "gcp" {
		t.Fatalf("expected gcp, got %s", got)
	}
}

func TestDetectProvider_GCPProjectsPath(t *testing.T) {
	r, _ := http.NewRequest(http.MethodGet, "https://cloudresourcemanager.googleapis.com/v1/projects/myproject", nil)

	got, err := auth.DetectProvider(r)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "gcp" {
		t.Fatalf("expected gcp, got %s", got)
	}
}

func TestDetectProvider_UnknownProvider(t *testing.T) {
	r, _ := http.NewRequest(http.MethodGet, "https://example.com/api/v2/resource", nil)

	_, err := auth.DetectProvider(r)
	if !errors.Is(err, auth.ErrUnknownProvider) {
		t.Fatalf("expected ErrUnknownProvider, got %v", err)
	}
}

func TestDetectProvider_EmptyRequest(t *testing.T) {
	r, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)

	_, err := auth.DetectProvider(r)
	if !errors.Is(err, auth.ErrUnknownProvider) {
		t.Fatalf("expected ErrUnknownProvider for empty request, got %v", err)
	}
}

func TestDetectProvider_AWSPriorityOverGCPPath(t *testing.T) {
	// X-Amz-Date があれば、GCP パスよりも AWS が優先される
	r, _ := http.NewRequest(http.MethodGet, "https://example.com/storage/v1/b/bucket", nil)
	r.Header.Set("X-Amz-Date", "20260315T000000Z")

	got, err := auth.DetectProvider(r)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "aws" {
		t.Fatalf("expected aws (higher priority), got %s", got)
	}
}

func TestDetectProvider_AuthorizationNotAWS4(t *testing.T) {
	// "AWS4-HMAC-SHA256" の prefix が一致しない場合は AWS と判定しない
	r, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	r.Header.Set("Authorization", "Basic dXNlcjpwYXNz")

	_, err := auth.DetectProvider(r)
	if !errors.Is(err, auth.ErrUnknownProvider) {
		t.Fatalf("expected ErrUnknownProvider for Basic auth, got %v", err)
	}
}
