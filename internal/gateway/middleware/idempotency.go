package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const idempotencyTTL = 5 * time.Minute

// idempotencyGCInterval is how often the GC runs to evict expired records.
const idempotencyGCInterval = 2 * time.Minute

// maxIdempotencyBodyBytes is the maximum number of bytes read from the request body for idempotency hashing.
const maxIdempotencyBodyBytes = 1 << 20 // 1 MB

// idempotencyRecord stores a cached response for an idempotency key.
type idempotencyRecord struct {
	bodyHash   string // SHA-256 of the request body at first call
	statusCode int
	body       []byte
	headers    map[string]string
	expiresAt  time.Time
}

// serviceNameContextKey is the context key type for the resolved service name.
type serviceNameContextKey struct{}

// ServiceNameKey is the context key used by the gateway handler to store the resolved service name.
// The idempotency middleware reads this value to scope idempotency keys per service.
var ServiceNameKey = serviceNameContextKey{}

// WithServiceName returns a copy of ctx with the resolved service name set.
// Call this in the gateway handler after the service name is resolved.
func WithServiceName(ctx context.Context, serviceName string) context.Context {
	return context.WithValue(ctx, ServiceNameKey, serviceName)
}

// ServiceNameFromContext returns the resolved service name stored in ctx, or "" if not set.
func ServiceNameFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ServiceNameKey).(string)
	return v
}

// IdempotencyStore is a thread-safe in-memory store for idempotency records.
// Keys are scoped per service: "<service>:<idempotency-key>".
// Expired records are purged periodically by a background goroutine started in NewIdempotencyStore.
type IdempotencyStore struct {
	mu      sync.Mutex
	records map[string]*idempotencyRecord
}

// NewIdempotencyStore creates a new IdempotencyStore and starts a background GC goroutine
// that evicts TTL-expired records every idempotencyGCInterval.
// The GC goroutine stops when ctx is cancelled.
func NewIdempotencyStore(ctx context.Context) *IdempotencyStore {
	s := &IdempotencyStore{
		records: make(map[string]*idempotencyRecord),
	}
	go s.runGC(ctx)
	return s
}

// runGC periodically removes expired records from the store.
// It stops when ctx is cancelled.
func (s *IdempotencyStore) runGC(ctx context.Context) {
	ticker := time.NewTicker(idempotencyGCInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.gc()
		}
	}
}

// gc removes all expired records. It is called by runGC on a timer.
func (s *IdempotencyStore) gc() {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, rec := range s.records {
		if now.After(rec.expiresAt) {
			delete(s.records, k)
		}
	}
}

func (s *IdempotencyStore) get(key string) (*idempotencyRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(rec.expiresAt) {
		delete(s.records, key)
		return nil, false
	}
	return rec, true
}

func (s *IdempotencyStore) set(key string, rec *idempotencyRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[key] = rec
}

// detectServiceFromRequest extracts the service name directly from the request.
// For AWS requests, the service is embedded in the Authorization header's credential scope.
// For GCP requests, it is inferred from the URL path prefix.
// As a final fallback it reads the X-Cloudia-Service header (useful for internal tooling).
func detectServiceFromRequest(r *http.Request) string {
	// AWS SigV4: Authorization: AWS4-HMAC-SHA256 Credential=key/date/region/SERVICE/aws4_request,...
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "AWS4-HMAC-SHA256 ") {
		rest := strings.TrimPrefix(authHeader, "AWS4-HMAC-SHA256 ")
		for _, part := range strings.Split(rest, ",") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "Credential=") {
				credential := strings.TrimPrefix(part, "Credential=")
				// credential: accessKey/date/region/service/aws4_request
				credParts := strings.SplitN(credential, "/", 5)
				if len(credParts) == 5 {
					return credParts[3]
				}
			}
		}
	}

	// GCP: infer from URL path prefix
	path := r.URL.Path
	switch {
	case strings.HasPrefix(path, "/storage/v1/") || strings.HasPrefix(path, "/upload/storage/v1/"):
		return "storage"
	case strings.HasPrefix(path, "/compute/v1/"):
		return "compute"
	case strings.HasPrefix(path, "/v1/projects/"):
		return "pubsub"
	}

	// Legacy fallback: X-Cloudia-Service header (not sent by AWS/GCP CLI, but used in tests)
	if svc := r.Header.Get("X-Cloudia-Service"); svc != "" {
		return svc
	}

	return "unknown"
}

// idempotencyCapturingWriter captures status code and body for caching.
type idempotencyCapturingWriter struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (w *idempotencyCapturingWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *idempotencyCapturingWriter) Write(b []byte) (int, error) {
	w.buf.Write(b)
	return w.ResponseWriter.Write(b)
}

// hashBody returns the SHA-256 hex digest of b.
func hashBody(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// extractIdempotencyKey extracts the idempotency key from the request headers.
// AWS uses "X-Amzn-Idempotency-Token"; GCP uses "X-Goog-Request-Id".
// The service name is resolved from the request context (set by the gateway handler),
// falling back to direct header/URL detection so that AWS CLI and GCP CLI requests
// (which do not send X-Cloudia-Service) are properly scoped.
// Returns ("", "") if no idempotency key is found.
func extractIdempotencyKey(r *http.Request) (key, service string) {
	// Resolve service name: context > request detection.
	svc := ServiceNameFromContext(r.Context())
	if svc == "" {
		svc = detectServiceFromRequest(r)
	}

	// GCP header
	if v := r.Header.Get("X-Goog-Request-Id"); v != "" {
		return v, svc
	}
	// AWS header
	if v := r.Header.Get("X-Amzn-Idempotency-Token"); v != "" {
		return v, svc
	}
	return "", ""
}

// Idempotency returns a middleware that enforces idempotent request handling.
// It reads the idempotency key from X-Goog-Request-Id (GCP) or X-Amzn-Idempotency-Token (AWS).
// If the same key is seen again with the same body, the cached response is replayed.
// If the key is seen again with a different body, IdempotentParameterMismatch is returned.
// The scope is per service (X-Cloudia-Service header) with TTL of 5 minutes.
func Idempotency(store *IdempotencyStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			idempKey, svc := extractIdempotencyKey(r)
			if idempKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Read body up to maxIdempotencyBodyBytes to prevent memory exhaustion,
			// then restore the original body for downstream handlers.
			bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, maxIdempotencyBodyBytes))
			if err != nil {
				http.Error(w, "failed to read request body", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

			storeKey := svc + ":" + idempKey
			bodyHash := hashBody(bodyBytes)

			if rec, ok := store.get(storeKey); ok {
				if rec.bodyHash != bodyHash {
					// Parameter mismatch
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_ = json.NewEncoder(w).Encode(map[string]string{
						"__type":  "IdempotentParameterMismatch",
						"message": "The request uses the same idempotency token as a previous request but with different parameters.",
					})
					return
				}
				// Replay cached response
				for k, v := range rec.headers {
					w.Header().Set(k, v)
				}
				w.WriteHeader(rec.statusCode)
				_, _ = w.Write(rec.body)
				return
			}

			cw := &idempotencyCapturingWriter{
				ResponseWriter: w,
				status:         http.StatusOK,
			}

			next.ServeHTTP(cw, r)

			// Cache the response headers we care about
			cachedHeaders := map[string]string{
				"Content-Type": cw.Header().Get("Content-Type"),
			}

			store.set(storeKey, &idempotencyRecord{
				bodyHash:   bodyHash,
				statusCode: cw.status,
				body:       cw.buf.Bytes(),
				headers:    cachedHeaders,
				expiresAt:  time.Now().Add(idempotencyTTL),
			})
		})
	}
}
