package gcp

import (
	"encoding/json"
	"net/http"
)

// EncodeJSONResponse はステータスコードと任意のボディを JSON としてエンコードし、
// Content-Type を application/json; charset=utf-8 に設定してレスポンスに書き込みます。
func EncodeJSONResponse(w http.ResponseWriter, statusCode int, body any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	return json.NewEncoder(w).Encode(body)
}
