package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HMasataka/cloudia/internal/config"
)

func makeRequest(authHeader string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if authHeader != "" {
		r.Header.Set("Authorization", authHeader)
	}
	return r
}

func TestSigV4Verifier_CanHandle(t *testing.T) {
	v := NewSigV4Verifier(config.AWSAuthConfig{AccessKey: "test"})

	tests := []struct {
		name       string
		authHeader string
		want       bool
	}{
		{
			name:       "AWS4-HMAC-SHA256 header returns true",
			authHeader: "AWS4-HMAC-SHA256 Credential=test/20260315/us-east-1/s3/aws4_request, SignedHeaders=host, Signature=abc123",
			want:       true,
		},
		{
			name:       "empty header returns false",
			authHeader: "",
			want:       false,
		},
		{
			name:       "Bearer token returns false",
			authHeader: "Bearer sometoken",
			want:       false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := makeRequest(tc.authHeader)
			got := v.CanHandle(r)
			if got != tc.want {
				t.Errorf("CanHandle() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSigV4Verifier_Verify_Success(t *testing.T) {
	v := NewSigV4Verifier(config.AWSAuthConfig{AccessKey: "mykey"})

	authHeader := "AWS4-HMAC-SHA256 Credential=mykey/20260315/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-date, Signature=deadsig"
	r := makeRequest(authHeader)

	result, err := v.Verify(r)
	if err != nil {
		t.Fatalf("Verify() unexpected error: %v", err)
	}
	if result.Provider != "aws" {
		t.Errorf("Provider = %q, want %q", result.Provider, "aws")
	}
	if result.Region != "us-east-1" {
		t.Errorf("Region = %q, want %q", result.Region, "us-east-1")
	}
	if result.Service != "s3" {
		t.Errorf("Service = %q, want %q", result.Service, "s3")
	}
}

func TestSigV4Verifier_Verify_EmptyAccessKeyConfig(t *testing.T) {
	v := NewSigV4Verifier(config.AWSAuthConfig{})

	authHeader := "AWS4-HMAC-SHA256 Credential=test/20260315/ap-northeast-1/ec2/aws4_request, SignedHeaders=host, Signature=sig"
	r := makeRequest(authHeader)

	_, err := v.Verify(r)
	if err == nil {
		t.Fatal("Verify() expected error for unconfigured AccessKey, got nil")
	}
}

func TestSigV4Verifier_Verify_AccessKeyMismatch(t *testing.T) {
	v := NewSigV4Verifier(config.AWSAuthConfig{AccessKey: "expected"})

	authHeader := "AWS4-HMAC-SHA256 Credential=wrong/20260315/us-east-1/s3/aws4_request, SignedHeaders=host, Signature=sig"
	r := makeRequest(authHeader)

	_, err := v.Verify(r)
	if err == nil {
		t.Fatal("Verify() expected error for mismatched access key, got nil")
	}
}

func TestSigV4Verifier_Verify_MalformedCredential(t *testing.T) {
	v := NewSigV4Verifier(config.AWSAuthConfig{AccessKey: "test"})

	// Credential の "/" が不足している
	authHeader := "AWS4-HMAC-SHA256 Credential=bad-credential, SignedHeaders=host, Signature=sig"
	r := makeRequest(authHeader)

	_, err := v.Verify(r)
	if err == nil {
		t.Fatal("Verify() expected error for malformed credential, got nil")
	}
}

func TestSigV4Verifier_Verify_MissingSignature(t *testing.T) {
	v := NewSigV4Verifier(config.AWSAuthConfig{AccessKey: "test"})

	// Signature フィールドがない
	authHeader := "AWS4-HMAC-SHA256 Credential=test/20260315/us-east-1/s3/aws4_request, SignedHeaders=host"
	r := makeRequest(authHeader)

	_, err := v.Verify(r)
	if err == nil {
		t.Fatal("Verify() expected error for missing Signature, got nil")
	}
}

func TestSigV4Verifier_Verify_InvalidScopeTerminator(t *testing.T) {
	v := NewSigV4Verifier(config.AWSAuthConfig{AccessKey: "test"})

	// aws4_request ではなく不正なターミネータ
	authHeader := "AWS4-HMAC-SHA256 Credential=test/20260315/us-east-1/s3/bad_terminator, SignedHeaders=host, Signature=sig"
	r := makeRequest(authHeader)

	_, err := v.Verify(r)
	if err == nil {
		t.Fatal("Verify() expected error for invalid scope terminator, got nil")
	}
}
