package aws

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteS3Error_statusCode(t *testing.T) {
	// Given
	w := httptest.NewRecorder()

	// When
	WriteS3Error(w, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist", "bucket-name", "req-123")

	// Then
	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestWriteS3Error_contentType(t *testing.T) {
	// Given
	w := httptest.NewRecorder()

	// When
	WriteS3Error(w, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist", "bucket-name", "req-123")

	// Then
	ct := w.Header().Get("Content-Type")
	if ct != "application/xml" {
		t.Errorf("unexpected Content-Type: %q", ct)
	}
}

func TestWriteS3Error_bodyStructure(t *testing.T) {
	// Given
	w := httptest.NewRecorder()

	// When
	WriteS3Error(w, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist", "bucket-name", "req-123")

	// Then
	body := w.Body.String()

	if !strings.HasPrefix(body, xml.Header) {
		t.Errorf("response body should start with XML declaration, got: %q", body)
	}
	if !strings.Contains(body, "<Error>") {
		t.Errorf("expected <Error> root element, got: %q", body)
	}
	if !strings.Contains(body, "<Code>NoSuchBucket</Code>") {
		t.Errorf("expected Code element, got: %q", body)
	}
	if !strings.Contains(body, "<Message>The specified bucket does not exist</Message>") {
		t.Errorf("expected Message element, got: %q", body)
	}
	if !strings.Contains(body, "<BucketName>bucket-name</BucketName>") {
		t.Errorf("expected BucketName element, got: %q", body)
	}
	if !strings.Contains(body, "<RequestId>req-123</RequestId>") {
		t.Errorf("expected RequestId element, got: %q", body)
	}
}

func TestWriteS3Error_parseable(t *testing.T) {
	// Given
	w := httptest.NewRecorder()

	// When
	WriteS3Error(w, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist", "bucket-name", "req-123")

	// Then
	body := strings.TrimPrefix(w.Body.String(), xml.Header)

	var resp S3ErrorResponse
	if err := xml.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("failed to parse S3ErrorResponse XML: %v", err)
	}

	if resp.Code != "NoSuchBucket" {
		t.Errorf("expected Code=NoSuchBucket, got %q", resp.Code)
	}
	if resp.Message != "The specified bucket does not exist" {
		t.Errorf("expected Message=The specified bucket does not exist, got %q", resp.Message)
	}
	if resp.BucketName != "bucket-name" {
		t.Errorf("expected BucketName=bucket-name, got %q", resp.BucketName)
	}
	if resp.RequestID != "req-123" {
		t.Errorf("expected RequestId=req-123, got %q", resp.RequestID)
	}
}
