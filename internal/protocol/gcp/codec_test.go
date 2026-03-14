package gcp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HMasataka/cloudia/internal/service"
)

func TestGCPCodecDecodeRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		path        string
		wantService string
		wantErr     bool
	}{
		{
			name:        "storage",
			path:        "/storage/v1/b/bucket",
			wantService: "storage",
		},
		{
			name:        "compute",
			path:        "/compute/v1/projects/proj/zones/us-central1-a/instances",
			wantService: "compute",
		},
		{
			name:    "unknown path",
			path:    "/unknown/api",
			wantErr: true,
		},
	}

	codec := &GCPCodec{}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Header.Set("Content-Type", "application/json")

			got, err := codec.DecodeRequest(req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DecodeRequest(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.Provider != "gcp" {
				t.Errorf("Provider = %q, want %q", got.Provider, "gcp")
			}
			if got.Service != tt.wantService {
				t.Errorf("Service = %q, want %q", got.Service, tt.wantService)
			}
		})
	}
}

func TestGCPCodecEncodeResponse(t *testing.T) {
	t.Parallel()

	codec := &GCPCodec{}
	w := httptest.NewRecorder()
	resp := service.Response{
		StatusCode:  http.StatusOK,
		Body:        []byte(`{"name":"bucket"}`),
		ContentType: "application/json",
	}
	codec.EncodeResponse(w, resp)

	if w.Code != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != `{"name":"bucket"}` {
		t.Errorf("Body = %q, want %q", w.Body.String(), `{"name":"bucket"}`)
	}
}

func TestGCPCodecEncodeError(t *testing.T) {
	t.Parallel()

	codec := &GCPCodec{}
	w := httptest.NewRecorder()
	codec.EncodeError(w, fmt.Errorf("not found"), "ignored-request-id")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	body := w.Body.String()
	if !strings.Contains(body, "not found") {
		t.Errorf("body does not contain error message: %s", body)
	}
	if !strings.Contains(body, "UNKNOWN") {
		t.Errorf("body does not contain grpc status: %s", body)
	}
}
