package aws

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDecodeRESTJSONRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		contentType string
		wantAction  string
		wantMethod  string
		wantBody    string
	}{
		{
			name:        "POST with JSON body",
			method:      http.MethodPost,
			path:        "/clusters",
			body:        `{"name":"test-cluster"}`,
			contentType: "application/json",
			wantAction:  "clusters",
			wantMethod:  http.MethodPost,
			wantBody:    `{"name":"test-cluster"}`,
		},
		{
			name:       "GET request no body",
			method:     http.MethodGet,
			path:       "/clusters/test",
			body:       "",
			wantAction: "clusters/test",
			wantMethod: http.MethodGet,
			wantBody:   "",
		},
		{
			name:       "DELETE request",
			method:     http.MethodDelete,
			path:       "/clusters/my-cluster",
			body:       "",
			wantAction: "clusters/my-cluster",
			wantMethod: http.MethodDelete,
			wantBody:   "",
		},
		{
			name:        "PUT with JSON body",
			method:      http.MethodPut,
			path:        "/clusters/my-cluster/nodegroups/ng-1",
			body:        `{"scalingConfig":{"minSize":1}}`,
			contentType: "application/json",
			wantAction:  "clusters/my-cluster/nodegroups/ng-1",
			wantMethod:  http.MethodPut,
			wantBody:    `{"scalingConfig":{"minSize":1}}`,
		},
		{
			name:       "root path becomes empty action",
			method:     http.MethodGet,
			path:       "/",
			body:       "",
			wantAction: "",
			wantMethod: http.MethodGet,
			wantBody:   "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var bodyReader *bytes.Reader
			if tt.body != "" {
				bodyReader = bytes.NewReader([]byte(tt.body))
			} else {
				bodyReader = bytes.NewReader(nil)
			}

			req := httptest.NewRequest(tt.method, tt.path, bodyReader)
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			got, err := DecodeRESTJSONRequest(req)
			if err != nil {
				t.Fatalf("DecodeRESTJSONRequest() error = %v", err)
			}

			if got.Provider != "aws" {
				t.Errorf("Provider = %q, want %q", got.Provider, "aws")
			}
			if got.Action != tt.wantAction {
				t.Errorf("Action = %q, want %q", got.Action, tt.wantAction)
			}
			if got.Method != tt.wantMethod {
				t.Errorf("Method = %q, want %q", got.Method, tt.wantMethod)
			}
			if string(got.Body) != tt.wantBody {
				t.Errorf("Body = %q, want %q", string(got.Body), tt.wantBody)
			}
		})
	}
}

func TestDecodeRESTJSONRequest_QueryParams(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/clusters?maxResults=10&nextToken=abc", nil)

	got, err := DecodeRESTJSONRequest(req)
	if err != nil {
		t.Fatalf("DecodeRESTJSONRequest() error = %v", err)
	}

	if got.Params["maxResults"] != "10" {
		t.Errorf("Params[maxResults] = %q, want %q", got.Params["maxResults"], "10")
	}
	if got.Params["nextToken"] != "abc" {
		t.Errorf("Params[nextToken] = %q, want %q", got.Params["nextToken"], "abc")
	}
}
