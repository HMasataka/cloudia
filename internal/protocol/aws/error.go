package aws

import (
	"encoding/xml"
	"net/http"
)

const errorResponseNamespace = "https://iam.amazonaws.com/doc/2010-05-08/"

// ErrorResponse は AWS 互換の XML エラーレスポンス構造体です。
type ErrorResponse struct {
	XMLName   xml.Name    `xml:"ErrorResponse"`
	Error     ErrorDetail `xml:"Error"`
	RequestID string      `xml:"RequestId"`
}

// ErrorDetail はエラーコードとメッセージを保持します。
type ErrorDetail struct {
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

// WriteError は AWS 互換 XML エラーレスポンスを HTTP レスポンスとして出力します。
func WriteError(w http.ResponseWriter, statusCode int, code string, message string, requestID string) {
	resp := ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
		RequestID: requestID,
	}
	EncodeXMLResponse(w, statusCode, resp, errorResponseNamespace)
}
