package gcp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HMasataka/cloudia/internal/protocol/gcp"
)

func TestHTTPStatusToGRPCStatus(t *testing.T) {
	cases := []struct {
		httpStatus int
		grpcStatus string
	}{
		{http.StatusBadRequest, "INVALID_ARGUMENT"},
		{http.StatusUnauthorized, "UNAUTHENTICATED"},
		{http.StatusForbidden, "PERMISSION_DENIED"},
		{http.StatusNotFound, "NOT_FOUND"},
		{http.StatusConflict, "ALREADY_EXISTS"},
		{http.StatusNotImplemented, "UNIMPLEMENTED"},
	}

	for _, tc := range cases {
		got, ok := gcp.HTTPStatusToGRPCStatus[tc.httpStatus]
		if !ok {
			t.Errorf("HTTPStatusToGRPCStatus[%d] not found", tc.httpStatus)
			continue
		}
		if got != tc.grpcStatus {
			t.Errorf("HTTPStatusToGRPCStatus[%d] = %q, want %q", tc.httpStatus, got, tc.grpcStatus)
		}
	}
}

func TestWriteError(t *testing.T) {
	t.Run("known status code", func(t *testing.T) {
		w := httptest.NewRecorder()
		gcp.WriteError(w, http.StatusNotImplemented, "not implemented yet")

		if w.Code != http.StatusNotImplemented {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusNotImplemented)
		}

		contentType := w.Header().Get("Content-Type")
		if contentType != "application/json; charset=utf-8" {
			t.Errorf("Content-Type = %q, want %q", contentType, "application/json; charset=utf-8")
		}

		var got gcp.GCPErrorResponse
		if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if got.Error.Code != http.StatusNotImplemented {
			t.Errorf("error.code = %d, want %d", got.Error.Code, http.StatusNotImplemented)
		}
		if got.Error.Message != "not implemented yet" {
			t.Errorf("error.message = %q, want %q", got.Error.Message, "not implemented yet")
		}
		if got.Error.Status != "UNIMPLEMENTED" {
			t.Errorf("error.status = %q, want %q", got.Error.Status, "UNIMPLEMENTED")
		}
	})

	t.Run("unknown status code falls back to UNKNOWN", func(t *testing.T) {
		w := httptest.NewRecorder()
		gcp.WriteError(w, http.StatusTeapot, "i am a teapot")

		var got gcp.GCPErrorResponse
		if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if got.Error.Status != "UNKNOWN" {
			t.Errorf("error.status = %q, want %q", got.Error.Status, "UNKNOWN")
		}
	})

	t.Run("400 maps to INVALID_ARGUMENT", func(t *testing.T) {
		w := httptest.NewRecorder()
		gcp.WriteError(w, http.StatusBadRequest, "bad input")

		var got gcp.GCPErrorResponse
		if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if got.Error.Status != "INVALID_ARGUMENT" {
			t.Errorf("error.status = %q, want %q", got.Error.Status, "INVALID_ARGUMENT")
		}
	})

	t.Run("404 maps to NOT_FOUND", func(t *testing.T) {
		w := httptest.NewRecorder()
		gcp.WriteError(w, http.StatusNotFound, "resource not found")

		var got gcp.GCPErrorResponse
		if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if got.Error.Status != "NOT_FOUND" {
			t.Errorf("error.status = %q, want %q", got.Error.Status, "NOT_FOUND")
		}
	})
}
