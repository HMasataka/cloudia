package gateway_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/HMasataka/cloudia/internal/auth"
	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/gateway"
	"github.com/HMasataka/cloudia/internal/protocol"
	awsprotocol "github.com/HMasataka/cloudia/internal/protocol/aws"
	gcpprotocol "github.com/HMasataka/cloudia/internal/protocol/gcp"
	"github.com/HMasataka/cloudia/internal/service"
)

// stubVerifier は指定した結果を返す Verifier スタブです。
type stubVerifier struct {
	canHandle  bool
	authResult auth.AuthResult
	err        error
}

func (v *stubVerifier) CanHandle(_ *http.Request) bool { return v.canHandle }
func (v *stubVerifier) Verify(_ *http.Request) (auth.AuthResult, error) {
	return v.authResult, v.err
}

// stubService は指定したレスポンスを返す Service スタブです。
type stubService struct {
	provider string
	name     string
	resp     service.Response
	err      error
}

func (s *stubService) Name() string     { return s.name }
func (s *stubService) Provider() string { return s.provider }
func (s *stubService) Init(_ context.Context, _ service.ServiceDeps) error { return nil }
func (s *stubService) HandleRequest(_ context.Context, _ service.Request) (service.Response, error) {
	return s.resp, s.err
}
func (s *stubService) SupportedActions() []string                    { return nil }
func (s *stubService) Health(_ context.Context) service.HealthStatus { return service.HealthStatus{} }
func (s *stubService) Shutdown(_ context.Context) error              { return nil }

func newTestHandler(
	verifiers map[string]auth.Verifier,
	codecs map[string]protocol.Codec,
	registry *service.Registry,
) *gateway.ServiceHandler {
	return gateway.NewServiceHandler(verifiers, codecs, registry, zap.NewNop())
}

func defaultCodecs() map[string]protocol.Codec {
	return map[string]protocol.Codec{
		"aws": &awsprotocol.AWSCodec{},
		"gcp": &gcpprotocol.GCPCodec{},
	}
}

func TestServiceHandler_NoAuthHeader_Returns400(t *testing.T) {
	// Given: request without any auth header
	registry := service.NewRegistry()
	h := newTestHandler(map[string]auth.Verifier{}, defaultCodecs(), registry)

	req := httptest.NewRequest(http.MethodGet, "/some/path", nil)
	w := httptest.NewRecorder()

	// When
	h.ServeHTTP(w, req)

	// Then: 400 returned
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestServiceHandler_AWSAuth_UnregisteredService_Returns501XML(t *testing.T) {
	// Given: AWS SigV4 auth passes, no registered services
	verifiers := map[string]auth.Verifier{
		"aws": &stubVerifier{
			canHandle:  true,
			authResult: auth.AuthResult{Provider: "aws", Service: "s3"},
		},
	}
	registry := service.NewRegistry()
	h := newTestHandler(verifiers, defaultCodecs(), registry)

	authHeader := "AWS4-HMAC-SHA256 Credential=test/20260315/us-east-1/s3/aws4_request, SignedHeaders=host, Signature=abc"
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("Action=ListBuckets&Version=2006-03-01"))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	// When
	h.ServeHTTP(w, req)

	// Then: 501 with XML body containing UnsupportedOperation
	if w.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "UnsupportedOperation") {
		t.Errorf("expected UnsupportedOperation in body, got: %s", body)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "xml") {
		t.Errorf("expected XML content-type, got: %s", ct)
	}
}

func TestServiceHandler_GCPAuth_UnregisteredService_Returns501JSON(t *testing.T) {
	// Given: Bearer token auth passes, no registered services
	verifiers := map[string]auth.Verifier{
		"gcp": &stubVerifier{
			canHandle:  true,
			authResult: auth.AuthResult{Provider: "gcp"},
		},
	}
	registry := service.NewRegistry()
	h := newTestHandler(verifiers, defaultCodecs(), registry)

	req := httptest.NewRequest(http.MethodGet, "/storage/v1/buckets", nil)
	req.Header.Set("Authorization", "Bearer sometoken")
	w := httptest.NewRecorder()

	// When
	h.ServeHTTP(w, req)

	// Then: 501 with JSON body containing UNIMPLEMENTED
	if w.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "UNIMPLEMENTED") {
		t.Errorf("expected UNIMPLEMENTED in body, got: %s", body)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "json") {
		t.Errorf("expected JSON content-type, got: %s", ct)
	}
}

func TestServiceHandler_AWSAuthFailure_Returns403XML(t *testing.T) {
	// Given: AWS auth that fails (access key mismatch)
	verifiers := map[string]auth.Verifier{
		"aws": &stubVerifier{
			canHandle: true,
			err:       errors.New("auth: SignatureDoesNotMatch: access key mismatch"),
		},
	}
	registry := service.NewRegistry()
	h := newTestHandler(verifiers, defaultCodecs(), registry)

	authHeader := "AWS4-HMAC-SHA256 Credential=wrong-key/20260315/us-east-1/s3/aws4_request, SignedHeaders=host, Signature=abc"
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", authHeader)
	w := httptest.NewRecorder()

	// When
	h.ServeHTTP(w, req)

	// Then: 403 with XML body containing SignatureDoesNotMatch
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "xml") {
		t.Errorf("expected XML content-type, got: %s", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "SignatureDoesNotMatch") {
		t.Errorf("expected SignatureDoesNotMatch in body, got: %s", body)
	}
}

func TestServiceHandler_AWSAuth_RegisteredService_Returns200(t *testing.T) {
	// Given: AWS auth passes, s3 service registered
	verifiers := map[string]auth.Verifier{
		"aws": &stubVerifier{
			canHandle:  true,
			authResult: auth.AuthResult{Provider: "aws", Service: "s3"},
		},
	}
	registry := service.NewRegistry()
	svc := &stubService{
		provider: "aws",
		name:     "s3",
		resp: service.Response{
			StatusCode:  http.StatusOK,
			Body:        []byte("<ok/>"),
			ContentType: "text/xml",
		},
	}
	if err := registry.Register(svc); err != nil {
		t.Fatal(err)
	}

	h := newTestHandler(verifiers, defaultCodecs(), registry)

	authHeader := "AWS4-HMAC-SHA256 Credential=test/20260315/us-east-1/s3/aws4_request, SignedHeaders=host, Signature=abc"
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("Action=ListBuckets&Version=2006-03-01"))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	// When
	h.ServeHTTP(w, req)

	// Then: 200 returned
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestServiceHandler_ResponseContainsRequestID(t *testing.T) {
	// Given: registered service that succeeds
	verifiers := map[string]auth.Verifier{
		"aws": &stubVerifier{
			canHandle:  true,
			authResult: auth.AuthResult{Provider: "aws", Service: "s3"},
		},
	}
	registry := service.NewRegistry()
	svc := &stubService{
		provider: "aws",
		name:     "s3",
		resp: service.Response{
			StatusCode:  http.StatusOK,
			ContentType: "text/xml",
		},
	}
	if err := registry.Register(svc); err != nil {
		t.Fatal(err)
	}

	h := newTestHandler(verifiers, defaultCodecs(), registry)

	authHeader := "AWS4-HMAC-SHA256 Credential=test/20260315/us-east-1/s3/aws4_request, SignedHeaders=host, Signature=abc"
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("Action=ListBuckets&Version=2006-03-01"))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	// When
	h.ServeHTTP(w, req)

	// Then: X-Request-Id header is set in response
	xRequestID := w.Header().Get("X-Request-Id")
	if xRequestID == "" {
		t.Error("expected X-Request-Id header to be set, got empty string")
	}
}

func TestServiceHandler_HealthEndpoint_NoAuth(t *testing.T) {
	// Given: mux with /health bypassing ServiceHandler
	registry := service.NewRegistry()
	serviceHandler := newTestHandler(map[string]auth.Verifier{}, defaultCodecs(), registry)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	})
	mux.Handle("/", serviceHandler)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	// When
	mux.ServeHTTP(w, req)

	// Then: 200 without requiring auth
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// stubProxyService は ProxyService を実装するスタブです。
type stubProxyService struct {
	stubService
	serveHTTPCalled bool
	statusCode      int
}

func (s *stubProxyService) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	s.serveHTTPCalled = true
	w.WriteHeader(s.statusCode)
}

func TestServiceHandler_ProxyService_ServesHTTPDirectly(t *testing.T) {
	// Given: AWS auth passes with service "s3", registered service implements ProxyService
	verifiers := map[string]auth.Verifier{
		"aws": &stubVerifier{
			canHandle:  true,
			authResult: auth.AuthResult{Provider: "aws", Service: "s3"},
		},
	}
	registry := service.NewRegistry()
	proxySvc := &stubProxyService{
		stubService: stubService{provider: "aws", name: "s3"},
		statusCode:  http.StatusOK,
	}
	if err := registry.Register(proxySvc); err != nil {
		t.Fatal(err)
	}

	h := newTestHandler(verifiers, defaultCodecs(), registry)

	authHeader := "AWS4-HMAC-SHA256 Credential=test/20260315/us-east-1/s3/aws4_request, SignedHeaders=host, Signature=abc"
	req := httptest.NewRequest(http.MethodPut, "/bucket/key", strings.NewReader("body"))
	req.Header.Set("Authorization", authHeader)
	w := httptest.NewRecorder()

	// When
	h.ServeHTTP(w, req)

	// Then: ProxyService.ServeHTTP is called and X-Request-Id header is set
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !proxySvc.serveHTTPCalled {
		t.Error("expected ProxyService.ServeHTTP to be called")
	}
	if xRequestID := w.Header().Get("X-Request-Id"); xRequestID == "" {
		t.Error("expected X-Request-Id header to be set, got empty string")
	}
}

func TestServiceHandler_ProxyService_NotFound_Returns501(t *testing.T) {
	// Given: AWS auth passes with service "s3", no service registered
	verifiers := map[string]auth.Verifier{
		"aws": &stubVerifier{
			canHandle:  true,
			authResult: auth.AuthResult{Provider: "aws", Service: "s3"},
		},
	}
	registry := service.NewRegistry()
	h := newTestHandler(verifiers, defaultCodecs(), registry)

	authHeader := "AWS4-HMAC-SHA256 Credential=test/20260315/us-east-1/s3/aws4_request, SignedHeaders=host, Signature=abc"
	req := httptest.NewRequest(http.MethodPut, "/bucket/key", nil)
	req.Header.Set("Authorization", authHeader)
	w := httptest.NewRecorder()

	// When
	h.ServeHTTP(w, req)

	// Then: 501 with UnsupportedOperation
	if w.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "UnsupportedOperation") {
		t.Errorf("expected UnsupportedOperation in body, got: %s", w.Body.String())
	}
}

func TestServiceHandler_EmptyAuthService_UsesCodecPath(t *testing.T) {
	// Given: GCP auth passes with empty service (Query/JSON protocol path)
	verifiers := map[string]auth.Verifier{
		"gcp": &stubVerifier{
			canHandle:  true,
			authResult: auth.AuthResult{Provider: "gcp"},
		},
	}
	registry := service.NewRegistry()
	h := newTestHandler(verifiers, defaultCodecs(), registry)

	req := httptest.NewRequest(http.MethodGet, "/storage/v1/buckets", nil)
	req.Header.Set("Authorization", "Bearer sometoken")
	w := httptest.NewRecorder()

	// When
	h.ServeHTTP(w, req)

	// Then: falls through to codec path, returns 501 (service not found via codec decode)
	if w.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", w.Code)
	}
}

func defaultRealVerifiers() map[string]auth.Verifier {
	return map[string]auth.Verifier{
		"aws": auth.NewSigV4Verifier(config.AWSAuthConfig{AccessKey: "test"}),
		"gcp": auth.NewOAuthVerifier(config.GCPAuthConfig{}),
	}
}

// TestIntegration_AWSQuery_AuthPass_UnsupportedOperationXML tests Case 1:
// SigV4-signed AWS Query request (Action=DescribeInstances) -> auth pass -> UnsupportedOperation XML error.
func TestIntegration_AWSQuery_AuthPass_UnsupportedOperationXML(t *testing.T) {
	// Given: real SigV4Verifier with access key "test", no services registered
	registry := service.NewRegistry()
	h := newTestHandler(defaultRealVerifiers(), defaultCodecs(), registry)

	authHeader := "AWS4-HMAC-SHA256 Credential=test/20260315/us-east-1/ec2/aws4_request, SignedHeaders=host, Signature=abc"
	body := strings.NewReader("Action=DescribeInstances&Version=2016-11-15")
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	// When
	h.ServeHTTP(w, req)

	// Then: 501 with XML UnsupportedOperation
	if w.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "UnsupportedOperation") {
		t.Errorf("expected UnsupportedOperation in body, got: %s", w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "xml") {
		t.Errorf("expected XML content-type, got: %s", ct)
	}
}

// TestIntegration_AWSJSON_AuthPass_UnsupportedOperationXML tests Case 2:
// SigV4-signed AWS JSON request (X-Amz-Target: DynamoDB_20120810.ListTables) -> auth pass -> UnsupportedOperation XML error.
func TestIntegration_AWSJSON_AuthPass_UnsupportedOperationXML(t *testing.T) {
	// Given: real SigV4Verifier with access key "test", no services registered
	registry := service.NewRegistry()
	h := newTestHandler(defaultRealVerifiers(), defaultCodecs(), registry)

	authHeader := "AWS4-HMAC-SHA256 Credential=test/20260315/us-east-1/dynamodb/aws4_request, SignedHeaders=host, Signature=abc"
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}"))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("X-Amz-Target", "DynamoDB_20120810.ListTables")
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	w := httptest.NewRecorder()

	// When
	h.ServeHTTP(w, req)

	// Then: 501 with XML UnsupportedOperation
	if w.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "UnsupportedOperation") {
		t.Errorf("expected UnsupportedOperation in body, got: %s", w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "xml") {
		t.Errorf("expected XML content-type, got: %s", ct)
	}
}

// TestIntegration_GCP_BearerToken_AuthPass_UnimplementedJSON tests Case 3:
// Bearer token GCP request (/storage/v1/b) -> auth pass -> UNIMPLEMENTED JSON error.
func TestIntegration_GCP_BearerToken_AuthPass_UnimplementedJSON(t *testing.T) {
	// Given: real OAuthVerifier, no services registered
	registry := service.NewRegistry()
	h := newTestHandler(defaultRealVerifiers(), defaultCodecs(), registry)

	req := httptest.NewRequest(http.MethodGet, "/storage/v1/b", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()

	// When
	h.ServeHTTP(w, req)

	// Then: 501 with JSON UNIMPLEMENTED
	if w.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "UNIMPLEMENTED") {
		t.Errorf("expected UNIMPLEMENTED in body, got: %s", w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "json") {
		t.Errorf("expected JSON content-type, got: %s", ct)
	}
}

// TestIntegration_AWS_InvalidAccessKey_Returns403SignatureDoesNotMatch tests Case 4:
// Invalid AccessKey -> 403 + SignatureDoesNotMatch XML error.
func TestIntegration_AWS_InvalidAccessKey_Returns403SignatureDoesNotMatch(t *testing.T) {
	// Given: real SigV4Verifier expecting "test", request uses "wrong-key"
	registry := service.NewRegistry()
	h := newTestHandler(defaultRealVerifiers(), defaultCodecs(), registry)

	authHeader := "AWS4-HMAC-SHA256 Credential=wrong-key/20260315/us-east-1/s3/aws4_request, SignedHeaders=host, Signature=abc"
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	// When
	h.ServeHTTP(w, req)

	// Then: 403 with XML SignatureDoesNotMatch
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "SignatureDoesNotMatch") {
		t.Errorf("expected SignatureDoesNotMatch in body, got: %s", w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "xml") {
		t.Errorf("expected XML content-type, got: %s", ct)
	}
}

// TestIntegration_GCP_EmptyBearerToken_Returns401 tests Case 7:
// Empty Bearer token -> 401 error.
func TestIntegration_GCP_EmptyBearerToken_Returns401(t *testing.T) {
	// Given: real OAuthVerifier, Bearer header with empty token
	registry := service.NewRegistry()
	h := newTestHandler(defaultRealVerifiers(), defaultCodecs(), registry)

	req := httptest.NewRequest(http.MethodGet, "/storage/v1/b", nil)
	req.Header.Set("Authorization", "Bearer ")
	w := httptest.NewRecorder()

	// When
	h.ServeHTTP(w, req)

	// Then: 401
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestServiceHandler_404Response_EmitsWarnLog verifies that when HandleRequest
// returns a 404 response, a Warn-level log containing provider/service/action/request_id is emitted.
func TestServiceHandler_404Response_EmitsWarnLog(t *testing.T) {
	// Given: AWS auth passes, s3 service returns 404
	core, logs := observer.New(zapcore.WarnLevel)
	logger := zap.New(core)

	verifiers := map[string]auth.Verifier{
		"aws": &stubVerifier{
			canHandle:  true,
			authResult: auth.AuthResult{Provider: "aws", Service: "s3"},
		},
	}
	registry := service.NewRegistry()
	svc := &stubService{
		provider: "aws",
		name:     "s3",
		resp: service.Response{
			StatusCode:  http.StatusNotFound,
			Body:        []byte("<Error><Code>NoSuchKey</Code></Error>"),
			ContentType: "text/xml",
		},
	}
	if err := registry.Register(svc); err != nil {
		t.Fatal(err)
	}

	h := gateway.NewServiceHandler(verifiers, defaultCodecs(), registry, logger)

	authHeader := "AWS4-HMAC-SHA256 Credential=test/20260315/us-east-1/s3/aws4_request, SignedHeaders=host, Signature=abc"
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("Action=GetObject&Version=2006-03-01"))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	// When
	h.ServeHTTP(w, req)

	// Then: response is 404
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	// And: a Warn log is emitted with required fields
	warnLogs := logs.FilterLevelExact(zapcore.WarnLevel).All()
	if len(warnLogs) == 0 {
		t.Fatal("expected at least one Warn log, got none")
	}

	var found bool
	for _, entry := range warnLogs {
		if entry.Message != "resource not found" {
			continue
		}
		fields := entry.ContextMap()
		if fields["provider"] == "" {
			t.Error("expected 'provider' field in warn log")
		}
		if fields["service"] == "" {
			t.Error("expected 'service' field in warn log")
		}
		if fields["action"] == "" {
			t.Error("expected 'action' field in warn log")
		}
		if fields["request_id"] == "" {
			t.Error("expected 'request_id' field in warn log")
		}
		found = true
		break
	}
	if !found {
		t.Errorf("expected Warn log with message 'resource not found', got: %+v", warnLogs)
	}
}
