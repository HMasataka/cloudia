package service

import "net/http"

// ProxyService は HTTP プロキシとして動作するサービスのインターフェースです。
type ProxyService interface {
	Service

	// ServeHTTP は HTTP リクエストをプロキシとして処理します。
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}
