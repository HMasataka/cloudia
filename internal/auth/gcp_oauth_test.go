package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HMasataka/cloudia/internal/auth"
	"github.com/HMasataka/cloudia/internal/config"
)

func newRequest(authHeader string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	return req
}

func TestOAuthVerifier_CanHandle(t *testing.T) {
	v := auth.NewOAuthVerifier(config.GCPAuthConfig{})

	tests := []struct {
		name       string
		authHeader string
		want       bool
	}{
		{
			name:       "Bearer トークンあり",
			authHeader: "Bearer mytoken",
			want:       true,
		},
		{
			name:       "Bearer のみ（トークンなし）",
			authHeader: "Bearer ",
			want:       true,
		},
		{
			name:       "Authorization ヘッダーなし",
			authHeader: "",
			want:       false,
		},
		{
			name:       "Basic 認証",
			authHeader: "Basic dXNlcjpwYXNz",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newRequest(tt.authHeader)
			got := v.CanHandle(req)
			if got != tt.want {
				t.Errorf("CanHandle() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOAuthVerifier_Verify(t *testing.T) {
	v := auth.NewOAuthVerifier(config.GCPAuthConfig{})

	t.Run("有効なトークン", func(t *testing.T) {
		req := newRequest("Bearer validtoken")
		result, err := v.Verify(req)
		if err != nil {
			t.Fatalf("Verify() error = %v, want nil", err)
		}
		if result.Provider != "gcp" {
			t.Errorf("Provider = %q, want %q", result.Provider, "gcp")
		}
	})

	t.Run("空のトークン", func(t *testing.T) {
		req := newRequest("Bearer ")
		_, err := v.Verify(req)
		if err == nil {
			t.Fatal("Verify() error = nil, want non-nil")
		}
	})

	t.Run("Authorization ヘッダーなし（トークン空）", func(t *testing.T) {
		req := newRequest("")
		_, err := v.Verify(req)
		if err == nil {
			t.Fatal("Verify() error = nil, want non-nil")
		}
	})
}
