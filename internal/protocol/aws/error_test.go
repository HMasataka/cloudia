package aws

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteError_statusCode(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, http.StatusBadRequest, "InvalidInput", "bad input", "req-123")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestWriteError_contentType(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, http.StatusNotFound, "NoSuchKey", "key not found", "req-456")

	ct := w.Header().Get("Content-Type")
	if ct != "text/xml; charset=utf-8" {
		t.Errorf("unexpected Content-Type: %q", ct)
	}
}

func TestWriteError_bodyStructure(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, http.StatusForbidden, "UnsupportedOperation", "not supported", "req-789")

	body := w.Body.String()

	// XML 宣言を含む
	if !strings.HasPrefix(body, xml.Header) {
		t.Errorf("response body should start with XML declaration, got: %q", body)
	}

	// ErrorResponse ルート要素
	if !strings.Contains(body, "<ErrorResponse") {
		t.Errorf("expected <ErrorResponse> element, got: %q", body)
	}

	// xmlns 属性
	if !strings.Contains(body, `xmlns="`) {
		t.Errorf("expected xmlns attribute, got: %q", body)
	}

	// Error/Code
	if !strings.Contains(body, "<Code>UnsupportedOperation</Code>") {
		t.Errorf("expected Code element, got: %q", body)
	}

	// Error/Message
	if !strings.Contains(body, "<Message>not supported</Message>") {
		t.Errorf("expected Message element, got: %q", body)
	}

	// RequestId
	if !strings.Contains(body, "<RequestId>req-789</RequestId>") {
		t.Errorf("expected RequestId element, got: %q", body)
	}
}

func TestWriteError_parseable(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, http.StatusInternalServerError, "InternalError", "internal", "req-000")

	// XML 宣言を除去して残りをパース
	body := strings.TrimPrefix(w.Body.String(), xml.Header)

	var resp ErrorResponse
	if err := xml.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("failed to parse ErrorResponse XML: %v", err)
	}

	if resp.Error.Code != "InternalError" {
		t.Errorf("expected Code=InternalError, got %q", resp.Error.Code)
	}
	if resp.Error.Message != "internal" {
		t.Errorf("expected Message=internal, got %q", resp.Error.Message)
	}
	if resp.RequestID != "req-000" {
		t.Errorf("expected RequestId=req-000, got %q", resp.RequestID)
	}
}
