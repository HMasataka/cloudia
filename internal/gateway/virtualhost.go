package gateway

import (
	"net/http"
	"strings"
)

// extractVirtualHostBucket は Host ヘッダーから S3 バーチャルホスト形式のバケット名を抽出します。
// `{bucket}.s3.localhost` パターンを検出します。ドットを含むバケット名はサポートしません。
func extractVirtualHostBucket(host string) (bucket string, ok bool) {
	// ポートを除去する
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}

	const suffix = ".s3.localhost"
	if !strings.HasSuffix(host, suffix) {
		return "", false
	}

	bucket = host[:len(host)-len(suffix)]
	if bucket == "" {
		return "", false
	}

	// ドットを含むバケット名はサポートしない（AWS S3 と同じ制約）
	if strings.Contains(bucket, ".") {
		return "", false
	}

	return bucket, true
}

// rewriteVirtualHostPath はバーチャルホスト形式のリクエストパスを
// `/{bucket}/{original-path}` に書き換えます。
func rewriteVirtualHostPath(r *http.Request) {
	bucket, ok := extractVirtualHostBucket(r.Host)
	if !ok {
		return
	}

	originalPath := r.URL.Path
	if !strings.HasPrefix(originalPath, "/") {
		originalPath = "/" + originalPath
	}

	r.URL.Path = "/" + bucket + originalPath
}
