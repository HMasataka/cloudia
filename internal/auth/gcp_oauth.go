package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/HMasataka/cloudia/internal/config"
)

// OAuthVerifier は GCP OAuth トークンを検証する Verifier 実装です。
type OAuthVerifier struct {
	cfg config.GCPAuthConfig
}

// NewOAuthVerifier は OAuthVerifier を生成して返します。
func NewOAuthVerifier(cfg config.GCPAuthConfig) *OAuthVerifier {
	return &OAuthVerifier{cfg: cfg}
}

// CanHandle は Authorization ヘッダーが "Bearer " で始まる場合に true を返します。
func (v *OAuthVerifier) CanHandle(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ")
}

// Verify は Bearer トークンを抽出して認証結果を返します。
// local モードではトークンが非空であれば AuthResult{Provider: "gcp"} を返します。
// トークンが空の場合は認証エラーを返します。
func (v *OAuthVerifier) Verify(r *http.Request) (AuthResult, error) {
	authHeader := r.Header.Get("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")

	if token == "" {
		return AuthResult{}, errors.New("auth: bearer token is empty")
	}

	return AuthResult{Provider: "gcp"}, nil
}
