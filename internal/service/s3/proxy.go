package s3

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/HMasataka/cloudia/internal/protocol/aws"
)

// responseRecorder wraps http.ResponseWriter and captures the status code.
// Body bytes are streamed directly to the underlying writer without buffering.
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

// parsePath splits an S3 request path into bucket and key components.
// "/" -> ("", ""), "/bucket" -> ("bucket", ""), "/bucket/key" -> ("bucket", "key").
func parsePath(path string) (bucket, key string) {
	trimmed := strings.TrimPrefix(path, "/")
	if trimmed == "" {
		return "", ""
	}
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

// ServeHTTP proxies the HTTP request to the MinIO backend and updates the State Store on success.
func (s *S3Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target, err := url.Parse(s.minio.baseURL)
	if err != nil {
		aws.WriteS3Error(w, http.StatusInternalServerError, "InternalError", "invalid minio endpoint", "", "")
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, proxyErr error) {
		bucket, _ := parsePath(req.URL.Path)
		aws.WriteS3Error(rw, http.StatusBadGateway, "ServiceUnavailable", fmt.Sprintf("minio unreachable: %s", proxyErr.Error()), bucket, "")
	}

	rec := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}
	proxy.ServeHTTP(rec, r)

	s.updateStateOnSuccess(r, rec.statusCode)
}
