package gcs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// createBucketRequest holds the GCS bucket creation request body.
type createBucketRequest struct {
	Name string `json:"name"`
}

// bufferedResponseRecorder buffers the status code and response body for XML→JSON conversion.
type bufferedResponseRecorder struct {
	header     http.Header
	body       bytes.Buffer
	statusCode int
}

func newBufferedResponseRecorder() *bufferedResponseRecorder {
	return &bufferedResponseRecorder{
		header:     make(http.Header),
		statusCode: http.StatusOK,
	}
}

func (r *bufferedResponseRecorder) Header() http.Header {
	return r.header
}

func (r *bufferedResponseRecorder) Write(b []byte) (int, error) {
	return r.body.Write(b)
}

func (r *bufferedResponseRecorder) WriteHeader(code int) {
	r.statusCode = code
}

// ServeHTTP translates GCS JSON API requests to S3/MinIO requests, proxies them,
// and converts XML responses back to GCS JSON.
func (s *GCSService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Reject resumable uploads immediately.
	if r.URL.Query().Get("uploadType") == "resumable" {
		writeGCSError(w, http.StatusNotImplemented, "Resumable uploads are not supported")
		return
	}

	op, bucket, key := parseGCSPath(r.URL.Path, r.Method)

	// Route to the appropriate handler.
	switch op {
	case opListBuckets:
		s.handleListBuckets(w, r)
	case opCreateBucket:
		s.handleCreateBucket(w, r)
	case opGetBucket:
		s.handleGetBucket(w, r, bucket)
	case opDeleteBucket:
		s.handleDeleteBucket(w, r, bucket)
	case opObject:
		s.handleObjectOperation(w, r, bucket, key)
	default:
		writeGCSError(w, http.StatusNotFound, "unknown GCS API path")
	}
}

// gcsOperation enumerates recognized GCS API operations.
type gcsOperation int

const (
	opUnknown      gcsOperation = iota
	opListBuckets               // GET /storage/v1/b
	opCreateBucket              // POST /storage/v1/b
	opGetBucket                 // GET /storage/v1/b/{bucket}
	opDeleteBucket              // DELETE /storage/v1/b/{bucket}
	opObject                    // all object-level paths
)

// parseGCSPath extracts the GCS operation, bucket name, and object key from the request.
func parseGCSPath(path, method string) (op gcsOperation, bucket, key string) {
	// Strip /storage/v1 or /upload/storage/v1 prefix.
	for _, prefix := range []string{"/upload/storage/v1", "/storage/v1"} {
		if strings.HasPrefix(path, prefix) {
			path = strings.TrimPrefix(path, prefix)
			break
		}
	}

	// path is now like /b, /b/{bucket}, /b/{bucket}/o, /b/{bucket}/o/{object}, etc.
	if path == "/b" || path == "/b/" {
		if method == http.MethodPost {
			return opCreateBucket, "", ""
		}
		return opListBuckets, "", ""
	}

	if !strings.HasPrefix(path, "/b/") {
		return opUnknown, "", ""
	}

	rest := strings.TrimPrefix(path, "/b/") // rest = "{bucket}" or "{bucket}/o/..."
	slashIdx := strings.IndexByte(rest, '/')
	if slashIdx < 0 {
		// Just /b/{bucket}
		if method == http.MethodDelete {
			return opDeleteBucket, rest, ""
		}
		return opGetBucket, rest, ""
	}

	bucket = rest[:slashIdx]
	afterBucket := rest[slashIdx:] // "/o", "/o/{key}", "/o/{key}/copyTo/..."

	if !strings.HasPrefix(afterBucket, "/o") {
		return opUnknown, bucket, ""
	}

	// Object-level operation.
	afterO := strings.TrimPrefix(afterBucket, "/o")
	if afterO == "" || afterO == "/" {
		key = ""
	} else {
		key = strings.TrimPrefix(afterO, "/")
	}

	return opObject, bucket, key
}

// handleListBuckets proxies GET /storage/v1/b → GET / and converts the XML response.
func (s *GCSService) handleListBuckets(w http.ResponseWriter, r *http.Request) {
	outReq := s.buildMinIORequest(r, http.MethodGet, "/", nil)
	if outReq == nil {
		writeGCSError(w, http.StatusInternalServerError, "failed to build backend request")
		return
	}

	rec := newBufferedResponseRecorder()
	proxy := s.newReverseProxy()
	proxy.ServeHTTP(rec, outReq)

	if rec.statusCode != http.StatusOK {
		s.forwardBufferedResponse(w, rec)
		return
	}

	jsonBody, err := convertListBucketsXMLToJSON(rec.body.Bytes(), requestBaseURL(r))
	if err != nil {
		s.logger.Sugar().Warnf("gcs: list buckets XML conversion failed: %v", err)
		// Return an empty list on conversion failure rather than an error.
		jsonBody, _ = json.Marshal(map[string]interface{}{"kind": "storage#buckets", "items": []interface{}{}})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBody) //nolint:errcheck
}

// handleCreateBucket proxies POST /storage/v1/b → PUT /{bucket}.
func (s *GCSService) handleCreateBucket(w http.ResponseWriter, r *http.Request) {
	// Read request body to get bucket name.
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeGCSError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	var req createBucketRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil || req.Name == "" {
		writeGCSError(w, http.StatusBadRequest, "missing or invalid bucket name in request body")
		return
	}

	outReq := s.buildMinIORequest(r, http.MethodPut, "/"+req.Name, nil)
	if outReq == nil {
		writeGCSError(w, http.StatusInternalServerError, "failed to build backend request")
		return
	}

	rec := newBufferedResponseRecorder()
	proxy := s.newReverseProxy()
	proxy.ServeHTTP(rec, outReq)

	if rec.statusCode != http.StatusOK && rec.statusCode != http.StatusCreated {
		s.forwardBufferedResponse(w, rec)
		return
	}

	s.updateStateOnSuccess(r, req.Name, http.MethodPost, rec.statusCode)

	jsonBody, _ := json.Marshal(gcsBucket{
		Kind:         "storage#bucket",
		ID:           req.Name,
		Name:         req.Name,
		SelfLink:     fmt.Sprintf("%s/storage/v1/b/%s", requestBaseURL(r), req.Name),
		TimeCreated:  "",
		Updated:      "",
		Location:     "US",
		StorageClass: "STANDARD",
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBody) //nolint:errcheck
}

// handleGetBucket proxies GET /storage/v1/b/{bucket} → HEAD /{bucket}.
func (s *GCSService) handleGetBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	outReq := s.buildMinIORequest(r, http.MethodHead, "/"+bucket, nil)
	if outReq == nil {
		writeGCSError(w, http.StatusInternalServerError, "failed to build backend request")
		return
	}

	rec := newBufferedResponseRecorder()
	proxy := s.newReverseProxy()
	proxy.ServeHTTP(rec, outReq)

	if rec.statusCode != http.StatusOK {
		s.forwardBufferedResponse(w, rec)
		return
	}

	jsonBody, err := convertBucketInfoToJSON(rec.header, bucket, requestBaseURL(r))
	if err != nil {
		writeGCSError(w, http.StatusInternalServerError, "failed to convert bucket info")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBody) //nolint:errcheck
}

// handleDeleteBucket proxies DELETE /storage/v1/b/{bucket} → DELETE /{bucket}.
func (s *GCSService) handleDeleteBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	outReq := s.buildMinIORequest(r, http.MethodDelete, "/"+bucket, nil)
	if outReq == nil {
		writeGCSError(w, http.StatusInternalServerError, "failed to build backend request")
		return
	}

	rec := newBufferedResponseRecorder()
	proxy := s.newReverseProxy()
	proxy.ServeHTTP(rec, outReq)

	if rec.statusCode == http.StatusNoContent || rec.statusCode == http.StatusOK {
		s.updateStateOnSuccess(r, bucket, http.MethodDelete, rec.statusCode)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	s.forwardBufferedResponse(w, rec)
}

// handleObjectOperation dispatches object-level GCS API operations to the appropriate handler.
func (s *GCSService) handleObjectOperation(w http.ResponseWriter, r *http.Request, bucket, key string) {
	// POST or PUT /upload/storage/v1/b/{bucket}/o[/{object}] — simple upload
	if strings.HasPrefix(r.URL.Path, "/upload/") && (r.Method == http.MethodPost || r.Method == http.MethodPut) {
		// Reject multipart/related uploads (resumable already rejected at top).
		ct := r.Header.Get("Content-Type")
		if strings.HasPrefix(ct, "multipart/related") {
			writeGCSError(w, http.StatusNotImplemented, "Multipart uploads are not supported")
			return
		}
		s.handleObjectUpload(w, r, bucket, key)
		return
	}

	// POST /storage/v1/b/{srcBucket}/o/{srcObject}/copyTo/b/{destBucket}/o/{destObject}
	if r.Method == http.MethodPost && strings.Contains(key, "/copyTo/") {
		s.handleObjectCopy(w, r, bucket, key)
		return
	}

	// GET /storage/v1/b/{bucket}/o — list objects
	if r.Method == http.MethodGet && key == "" {
		s.handleListObjects(w, r, bucket)
		return
	}

	// DELETE /storage/v1/b/{bucket}/o/{object}
	if r.Method == http.MethodDelete && key != "" {
		s.handleObjectDelete(w, r, bucket, key)
		return
	}

	// GET /storage/v1/b/{bucket}/o/{object}?alt=media — stream object data
	if r.Method == http.MethodGet && key != "" && r.URL.Query().Get("alt") == "media" {
		s.handleObjectDownload(w, r, bucket, key)
		return
	}

	// GET /storage/v1/b/{bucket}/o/{object} — object metadata
	if r.Method == http.MethodGet && key != "" {
		s.handleObjectMetadata(w, r, bucket, key)
		return
	}

	writeGCSError(w, http.StatusMethodNotAllowed, "method not allowed for this resource")
}

// handleObjectUpload proxies POST/PUT /upload/storage/v1/b/{bucket}/o[/{object}] → PUT /{bucket}/{object}.
// The object name is taken from the path key when present, otherwise from the "name" query parameter.
func (s *GCSService) handleObjectUpload(w http.ResponseWriter, r *http.Request, bucket, key string) {
	objectName := key
	if objectName == "" {
		objectName = r.URL.Query().Get("name")
	}
	if objectName == "" {
		writeGCSError(w, http.StatusBadRequest, "missing required query parameter: name")
		return
	}

	s3Path := "/" + bucket + "/" + objectName

	// Read the body to sign correctly.
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeGCSError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	outReq := s.buildMinIORequest(r, http.MethodPut, s3Path, bodyBytes)
	if outReq == nil {
		writeGCSError(w, http.StatusInternalServerError, "failed to build backend request")
		return
	}
	// Keep Content-Type from the incoming request.
	outReq.Header.Set("Content-Type", r.Header.Get("Content-Type"))

	rec := newBufferedResponseRecorder()
	s.newReverseProxy().ServeHTTP(rec, outReq)

	if rec.statusCode != http.StatusOK && rec.statusCode != http.StatusCreated {
		s.forwardBufferedResponse(w, rec)
		return
	}

	jsonBody := convertObjectUploadToJSON(bucket, objectName, rec.header, requestBaseURL(r))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBody) //nolint:errcheck
}

// handleObjectDownload streams GET /{bucket}/{object} response directly to the client.
func (s *GCSService) handleObjectDownload(w http.ResponseWriter, r *http.Request, bucket, key string) {
	s3Path := "/" + bucket + "/" + key

	outReq := s.buildMinIORequest(r, http.MethodGet, s3Path, nil)
	if outReq == nil {
		writeGCSError(w, http.StatusInternalServerError, "failed to build backend request")
		return
	}

	// Stream without buffering.
	s.newReverseProxy().ServeHTTP(w, outReq)
}

// handleObjectMetadata proxies GET /storage/v1/b/{bucket}/o/{object} → HEAD /{bucket}/{object}
// and converts the response headers to GCS JSON.
func (s *GCSService) handleObjectMetadata(w http.ResponseWriter, r *http.Request, bucket, key string) {
	s3Path := "/" + bucket + "/" + key

	outReq := s.buildMinIORequest(r, http.MethodHead, s3Path, nil)
	if outReq == nil {
		writeGCSError(w, http.StatusInternalServerError, "failed to build backend request")
		return
	}

	rec := newBufferedResponseRecorder()
	s.newReverseProxy().ServeHTTP(rec, outReq)

	if rec.statusCode != http.StatusOK {
		s.forwardBufferedResponse(w, rec)
		return
	}

	jsonBody := convertObjectMetadataToJSON(bucket, key, rec.header, requestBaseURL(r))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBody) //nolint:errcheck
}

// handleObjectDelete proxies DELETE /storage/v1/b/{bucket}/o/{object} → DELETE /{bucket}/{object}.
func (s *GCSService) handleObjectDelete(w http.ResponseWriter, r *http.Request, bucket, key string) {
	s3Path := "/" + bucket + "/" + key

	outReq := s.buildMinIORequest(r, http.MethodDelete, s3Path, nil)
	if outReq == nil {
		writeGCSError(w, http.StatusInternalServerError, "failed to build backend request")
		return
	}

	rec := newBufferedResponseRecorder()
	s.newReverseProxy().ServeHTTP(rec, outReq)

	if rec.statusCode == http.StatusNoContent || rec.statusCode == http.StatusOK {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	s.forwardBufferedResponse(w, rec)
}

// handleListObjects proxies GET /storage/v1/b/{bucket}/o → GET /{bucket}?list-type=2
// and converts the XML response to GCS JSON.
func (s *GCSService) handleListObjects(w http.ResponseWriter, r *http.Request, bucket string) {
	s3Path := "/" + bucket

	outReq := s.buildMinIORequest(r, http.MethodGet, s3Path, nil)
	if outReq == nil {
		writeGCSError(w, http.StatusInternalServerError, "failed to build backend request")
		return
	}

	// Set S3 list-type=2 query param; forward GCS prefix/maxResults as S3 prefix/max-keys.
	q := outReq.URL.Query()
	q.Set("list-type", "2")
	if prefix := r.URL.Query().Get("prefix"); prefix != "" {
		q.Set("prefix", prefix)
	}
	if maxResults := r.URL.Query().Get("maxResults"); maxResults != "" {
		q.Set("max-keys", maxResults)
	}
	if pageToken := r.URL.Query().Get("pageToken"); pageToken != "" {
		q.Set("continuation-token", pageToken)
	}
	outReq.URL.RawQuery = q.Encode()

	rec := newBufferedResponseRecorder()
	s.newReverseProxy().ServeHTTP(rec, outReq)

	if rec.statusCode != http.StatusOK {
		s.forwardBufferedResponse(w, rec)
		return
	}

	jsonBody, err := convertListObjectsXMLToJSON(rec.body.Bytes(), bucket, requestBaseURL(r))
	if err != nil {
		s.logger.Sugar().Warnf("gcs: list objects XML conversion failed: %v", err)
		jsonBody, _ = json.Marshal(map[string]interface{}{"kind": "storage#objects", "items": []interface{}{}})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBody) //nolint:errcheck
}

// handleObjectCopy proxies POST .../copyTo/... → PUT /{destBucket}/{destObject} with x-amz-copy-source header.
// key contains "{srcObject}/copyTo/b/{destBucket}/o/{destObject}".
func (s *GCSService) handleObjectCopy(w http.ResponseWriter, r *http.Request, srcBucket, key string) {
	// Parse: "{srcObject}/copyTo/b/{destBucket}/o/{destObject}"
	const copyToMarker = "/copyTo/b/"
	idx := strings.Index(key, copyToMarker)
	if idx < 0 {
		writeGCSError(w, http.StatusBadRequest, "invalid copyTo path")
		return
	}

	srcObject := key[:idx]
	rest := key[idx+len(copyToMarker):] // "{destBucket}/o/{destObject}"

	slashIdx := strings.IndexByte(rest, '/')
	if slashIdx < 0 {
		writeGCSError(w, http.StatusBadRequest, "invalid copyTo path: missing dest object")
		return
	}

	destBucket := rest[:slashIdx]
	afterDestBucket := rest[slashIdx:] // "/o/{destObject}"

	if !strings.HasPrefix(afterDestBucket, "/o/") {
		writeGCSError(w, http.StatusBadRequest, "invalid copyTo path: expected /o/ segment")
		return
	}

	destObject := strings.TrimPrefix(afterDestBucket, "/o/")
	if destObject == "" {
		writeGCSError(w, http.StatusBadRequest, "invalid copyTo path: empty dest object name")
		return
	}

	s3DestPath := "/" + destBucket + "/" + destObject
	outReq := s.buildMinIORequest(r, http.MethodPut, s3DestPath, nil)
	if outReq == nil {
		writeGCSError(w, http.StatusInternalServerError, "failed to build backend request")
		return
	}

	outReq.Header.Set("x-amz-copy-source", "/"+srcBucket+"/"+srcObject)
	outReq.Body = http.NoBody
	outReq.ContentLength = 0

	rec := newBufferedResponseRecorder()
	s.newReverseProxy().ServeHTTP(rec, outReq)

	if rec.statusCode != http.StatusOK && rec.statusCode != http.StatusCreated {
		s.forwardBufferedResponse(w, rec)
		return
	}

	jsonBody := convertObjectUploadToJSON(destBucket, destObject, rec.header, requestBaseURL(r))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBody) //nolint:errcheck
}

// buildMinIORequest creates a new HTTP request targeting MinIO at the given path.
// The original GCS Authorization header is removed and replaced with SigV4.
func (s *GCSService) buildMinIORequest(r *http.Request, method, path string, body []byte) *http.Request {
	target, err := url.Parse(s.baseURL + path)
	if err != nil {
		return nil
	}

	outReq, err := http.NewRequestWithContext(r.Context(), method, target.String(), http.NoBody)
	if err != nil {
		return nil
	}

	// Copy safe headers, excluding Authorization (we will regenerate it).
	for k, vv := range r.Header {
		k := http.CanonicalHeaderKey(k)
		if k == "Authorization" || k == "Host" {
			continue
		}
		for _, v := range vv {
			outReq.Header.Add(k, v)
		}
	}

	outReq.Host = target.Host
	outReq.URL.Host = target.Host
	outReq.URL.Scheme = target.Scheme

	// Set content-length for the outgoing request body.
	if len(body) > 0 {
		outReq.Body = io.NopCloser(bytes.NewReader(body))
		outReq.ContentLength = int64(len(body))
	}

	signRequest(outReq, s.cfg.AccessKey, s.cfg.SecretKey, body)

	return outReq
}

// newReverseProxy creates a reverse proxy targeting the MinIO backend.
func (s *GCSService) newReverseProxy() *httputil.ReverseProxy {
	target, _ := url.Parse(s.baseURL)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Director = func(req *http.Request) {
		// Director is a no-op; buildMinIORequest has already set the correct URL.
	}
	return proxy
}

// forwardBufferedResponse writes a buffered recorder's response to the real ResponseWriter.
func (s *GCSService) forwardBufferedResponse(w http.ResponseWriter, rec *bufferedResponseRecorder) {
	for k, vv := range rec.header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(rec.statusCode)
	w.Write(rec.body.Bytes()) //nolint:errcheck
}

// requestBaseURL returns a base URL derived from the incoming GCS request's Host header.
func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

// writeGCSError writes a minimal GCS-compatible JSON error response.
func writeGCSError(w http.ResponseWriter, statusCode int, message string) {
	body, _ := json.Marshal(map[string]interface{}{
		"error": map[string]interface{}{
			"code":    statusCode,
			"message": message,
			"errors": []map[string]string{
				{"message": message, "domain": "global", "reason": "backendError"},
			},
		},
	})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(body) //nolint:errcheck
}
