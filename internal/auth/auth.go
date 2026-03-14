package auth

import "net/http"

// AuthResult は認証・認可の結果を保持する構造体です。
type AuthResult struct {
	Provider  string
	Region    string
	Service   string
	AccountID string
}

// Verifier はリクエストの認証を行うインターフェースです。
type Verifier interface {
	// Verify はリクエストを検証し、AuthResult を返します。
	Verify(*http.Request) (AuthResult, error)

	// CanHandle はこの Verifier がリクエストを処理できるかどうかを返します。
	CanHandle(*http.Request) bool
}
