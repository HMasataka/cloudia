package aws

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type sampleBody struct {
	XMLName xml.Name `xml:"Sample"`
	Value   string   `xml:"Value"`
}

func TestEncodeXMLResponse_statusAndContentType(t *testing.T) {
	w := httptest.NewRecorder()
	EncodeXMLResponse(w, http.StatusOK, sampleBody{Value: "hello"}, "")

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "text/xml; charset=utf-8" {
		t.Errorf("unexpected Content-Type: %q", ct)
	}
}

func TestEncodeXMLResponse_bodyContainsXMLHeader(t *testing.T) {
	w := httptest.NewRecorder()
	EncodeXMLResponse(w, http.StatusOK, sampleBody{Value: "world"}, "")

	body := w.Body.String()
	if !strings.HasPrefix(body, xml.Header) {
		t.Errorf("response body should start with XML declaration, got: %q", body)
	}
}

func TestEncodeXMLResponse_withNamespace(t *testing.T) {
	w := httptest.NewRecorder()
	EncodeXMLResponse(w, http.StatusCreated, sampleBody{Value: "ns"}, "https://example.com/ns")

	body := w.Body.String()
	if !strings.Contains(body, `xmlns="https://example.com/ns"`) {
		t.Errorf("expected xmlns attribute in body, got: %q", body)
	}
	if !strings.Contains(body, "<Value>ns</Value>") {
		t.Errorf("expected value in body, got: %q", body)
	}
	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}
}

func TestEncodeXMLResponse_withoutNamespace(t *testing.T) {
	w := httptest.NewRecorder()
	EncodeXMLResponse(w, http.StatusOK, sampleBody{Value: "nons"}, "")

	body := w.Body.String()
	if strings.Contains(body, "xmlns") {
		t.Errorf("expected no xmlns attribute, got: %q", body)
	}
}
