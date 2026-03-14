package auth

import (
	"errors"
	"net/http"
	"strings"
)

// ErrUnknownProvider はリクエストからプロバイダを特定できない場合に返されるエラーです。
var ErrUnknownProvider = errors.New("unknown provider")

// DetectProvider はリクエストのヘッダーと URL パスを検査し、クラウドプロバイダ名を返します。
// 検出優先順位:
//  1. Authorization ヘッダーが "AWS4-HMAC-SHA256 " で始まる -> "aws"
//  2. X-Amz-Target ヘッダーが存在する -> "aws"
//  3. X-Amz-Date ヘッダーが存在する -> "aws"
//  4. Authorization ヘッダーが "Bearer " で始まる -> "gcp"
//  5. URL パスが GCP パスプレフィックスにマッチする -> "gcp"
//  6. いずれにも該当しない -> ErrUnknownProvider
func DetectProvider(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")

	if strings.HasPrefix(authHeader, "AWS4-HMAC-SHA256 ") {
		return "aws", nil
	}

	if r.Header.Get("X-Amz-Target") != "" {
		return "aws", nil
	}

	if r.Header.Get("X-Amz-Date") != "" {
		return "aws", nil
	}

	if strings.HasPrefix(authHeader, "Bearer ") {
		return "gcp", nil
	}

	if isGCPPath(r.URL.Path) {
		return "gcp", nil
	}

	return "", ErrUnknownProvider
}

var gcpPathPrefixes = []string{
	"/storage/v1/",
	"/compute/v1/",
	"/v1/projects/",
}

func isGCPPath(path string) bool {
	for _, prefix := range gcpPathPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
