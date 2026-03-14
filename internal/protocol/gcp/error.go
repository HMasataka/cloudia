package gcp

import "net/http"

// HTTPStatusToGRPCStatus は HTTP ステータスコードを GCP/gRPC ステータス文字列にマッピングします。
var HTTPStatusToGRPCStatus = map[int]string{
	http.StatusBadRequest:   "INVALID_ARGUMENT",
	http.StatusUnauthorized: "UNAUTHENTICATED",
	http.StatusForbidden:    "PERMISSION_DENIED",
	http.StatusNotFound:     "NOT_FOUND",
	http.StatusConflict:     "ALREADY_EXISTS",
	http.StatusNotImplemented: "UNIMPLEMENTED",
}

// gcpErrorDetail は GCP 互換エラーレスポンスの error フィールドを表します。
type gcpErrorDetail struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// GCPErrorResponse は GCP 互換 JSON エラーレスポンスの構造体です。
type GCPErrorResponse struct {
	Error gcpErrorDetail `json:"error"`
}

// WriteError は GCP 互換 JSON エラーレスポンスを書き込みます。
// 出力形式: {"error": {"code": <statusCode>, "message": "<message>", "status": "<grpcStatus>"}}
// statusCode に対応する gRPC ステータスが未定義の場合は "UNKNOWN" を使用します。
func WriteError(w http.ResponseWriter, statusCode int, message string) {
	grpcStatus, ok := HTTPStatusToGRPCStatus[statusCode]
	if !ok {
		grpcStatus = "UNKNOWN"
	}

	resp := GCPErrorResponse{
		Error: gcpErrorDetail{
			Code:    statusCode,
			Message: message,
			Status:  grpcStatus,
		},
	}

	_ = EncodeJSONResponse(w, statusCode, resp)
}
