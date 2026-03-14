package aws

import (
	"encoding/xml"
	"net/http"
)

// S3ErrorResponse は S3 互換 XML エラーレスポンスのルート要素です。
type S3ErrorResponse struct {
	XMLName    xml.Name `xml:"Error"`
	Code       string   `xml:"Code"`
	Message    string   `xml:"Message"`
	BucketName string   `xml:"BucketName"`
	RequestID  string   `xml:"RequestId"`
}

// WriteS3Error は S3 互換 XML エラーレスポンスを HTTP レスポンスとして出力します。
func WriteS3Error(w http.ResponseWriter, statusCode int, code string, message string, bucketName string, requestID string) {
	resp := S3ErrorResponse{
		Code:       code,
		Message:    message,
		BucketName: bucketName,
		RequestID:  requestID,
	}

	data, err := xml.Marshal(resp)
	if err != nil {
		http.Error(w, "xml encoding error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(data)
}
