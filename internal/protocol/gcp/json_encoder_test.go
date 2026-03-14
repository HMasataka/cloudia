package gcp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HMasataka/cloudia/internal/protocol/gcp"
)

func TestEncodeJSONResponse(t *testing.T) {
	t.Run("sets Content-Type header", func(t *testing.T) {
		w := httptest.NewRecorder()
		gcp.EncodeJSONResponse(w, http.StatusOK, map[string]string{"key": "value"})

		got := w.Header().Get("Content-Type")
		want := "application/json; charset=utf-8"
		if got != want {
			t.Errorf("Content-Type = %q, want %q", got, want)
		}
	})

	t.Run("writes status code", func(t *testing.T) {
		w := httptest.NewRecorder()
		gcp.EncodeJSONResponse(w, http.StatusCreated, nil)

		if w.Code != http.StatusCreated {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusCreated)
		}
	})

	t.Run("encodes body as JSON", func(t *testing.T) {
		w := httptest.NewRecorder()
		body := map[string]string{"hello": "world"}
		gcp.EncodeJSONResponse(w, http.StatusOK, body)

		var got map[string]string
		if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
			t.Fatalf("failed to decode response body: %v", err)
		}
		if got["hello"] != "world" {
			t.Errorf("body[hello] = %q, want %q", got["hello"], "world")
		}
	})
}
