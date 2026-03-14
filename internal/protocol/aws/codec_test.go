package aws

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HMasataka/cloudia/internal/service"
)

func TestAWSCodecDecodeRequest_JSON(t *testing.T) {
	t.Parallel()

	codec := &AWSCodec{}
	body := `{"TableName":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	req.Header.Set("X-Amz-Target", "DynamoDB_20120810.PutItem")
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")

	got, err := codec.DecodeRequest(req)
	if err != nil {
		t.Fatalf("DecodeRequest() error = %v", err)
	}
	if got.Provider != "aws" {
		t.Errorf("Provider = %q, want %q", got.Provider, "aws")
	}
	if got.Service != "dynamodb" {
		t.Errorf("Service = %q, want %q", got.Service, "dynamodb")
	}
	if got.Action != "PutItem" {
		t.Errorf("Action = %q, want %q", got.Action, "PutItem")
	}
}

func TestAWSCodecDecodeRequest_Query(t *testing.T) {
	t.Parallel()

	codec := &AWSCodec{}
	body := "Action=DescribeInstances&Version=2016-11-15&InstanceId.1=i-1234567890abcdef0"
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	got, err := codec.DecodeRequest(req)
	if err != nil {
		t.Fatalf("DecodeRequest() error = %v", err)
	}
	if got.Provider != "aws" {
		t.Errorf("Provider = %q, want %q", got.Provider, "aws")
	}
	if got.Action != "DescribeInstances" {
		t.Errorf("Action = %q, want %q", got.Action, "DescribeInstances")
	}
}

func TestAWSCodecDecodeRequest_RESTJSON(t *testing.T) {
	t.Parallel()

	codec := &AWSCodec{}
	body := `{"name":"test-cluster"}`
	req := httptest.NewRequest(http.MethodPost, "/clusters", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	got, err := codec.DecodeRequest(req)
	if err != nil {
		t.Fatalf("DecodeRequest() error = %v", err)
	}
	if got.Provider != "aws" {
		t.Errorf("Provider = %q, want %q", got.Provider, "aws")
	}
	if got.Action != "clusters" {
		t.Errorf("Action = %q, want %q", got.Action, "clusters")
	}
	if got.Method != http.MethodPost {
		t.Errorf("Method = %q, want %q", got.Method, http.MethodPost)
	}
}

func TestAWSCodecEncodeResponse(t *testing.T) {
	t.Parallel()

	codec := &AWSCodec{}
	w := httptest.NewRecorder()
	resp := service.Response{
		StatusCode:  http.StatusOK,
		Body:        []byte(`<result/>`),
		ContentType: "text/xml",
	}
	codec.EncodeResponse(w, resp)

	if w.Code != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/xml" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/xml")
	}
	if w.Body.String() != `<result/>` {
		t.Errorf("Body = %q, want %q", w.Body.String(), `<result/>`)
	}
}

func TestAWSCodecEncodeResponse_ExplicitStatus(t *testing.T) {
	t.Parallel()

	codec := &AWSCodec{}
	w := httptest.NewRecorder()
	resp := service.Response{
		StatusCode: http.StatusOK,
		Body:       []byte("ok"),
	}
	codec.EncodeResponse(w, resp)

	if w.Code != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAWSCodecEncodeError(t *testing.T) {
	t.Parallel()

	codec := &AWSCodec{}
	w := httptest.NewRecorder()
	codec.EncodeError(w, errTest("something went wrong"), "req-123")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	body := w.Body.String()
	if !strings.Contains(body, "InternalError") {
		t.Errorf("body does not contain error code: %s", body)
	}
	if !strings.Contains(body, "req-123") {
		t.Errorf("body does not contain requestID: %s", body)
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }
