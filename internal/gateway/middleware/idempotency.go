package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

const idempotencyTTL = 5 * time.Minute

// idempotencyRecord stores a cached response for an idempotency key.
type idempotencyRecord struct {
	bodyHash   string // SHA-256 of the request body at first call
	statusCode int
	body       []byte
	headers    map[string]string
	expiresAt  time.Time
}

// IdempotencyStore is a thread-safe in-memory store for idempotency records.
// Keys are scoped per service: "<service>:<idempotency-key>".
type IdempotencyStore struct {
	mu      sync.Mutex
	records map[string]*idempotencyRecord
}

// NewIdempotencyStore creates a new IdempotencyStore.
func NewIdempotencyStore() *IdempotencyStore {
	return &IdempotencyStore{
		records: make(map[string]*idempotencyRecord),
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
// AWS uses "X-Amzn-Idempotency-Token" or the body "ClientToken" field;
// GCP uses "X-Goog-Request-Id".
// Returns ("", "") if no key is found.
func extractIdempotencyKey(r *http.Request) (key, service string) {
	// GCP header
	if v := r.Header.Get("X-Goog-Request-Id"); v != "" {
		svc := r.Header.Get("X-Cloudia-Service")
		if svc == "" {
			svc = "unknown"
		}
		return v, svc
	}
	// AWS header variants
	if v := r.Header.Get("X-Amzn-Idempotency-Token"); v != "" {
		svc := r.Header.Get("X-Cloudia-Service")
		if svc == "" {
			svc = "unknown"
		}
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

			// Read body (need to restore it for the next handler)
			bodyBytes, err := io.ReadAll(r.Body)
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
