package middleware

import (
	"net/http"
	"time"
)

// Timeout は指定された期間でリクエストをタイムアウトさせるミドルウェアを返します。
// タイムアウト時は 503 と {"error": "request timeout"} を返します。
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.TimeoutHandler(next, d, `{"error": "request timeout"}`)
	}
}
