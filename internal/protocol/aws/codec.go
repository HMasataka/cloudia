package aws

import (
	"errors"
	"net/http"
	"strings"

	"github.com/HMasataka/cloudia/internal/service"
)

// AWSCodec は AWS プロバイダ用の protocol.Codec 実装です。
type AWSCodec struct{}

// DecodeRequest は HTTP リクエストを service.Request に変換します。
// 1. X-Amz-Target ヘッダーがあれば JSON Target プロトコルとして処理します。
// 2. Content-Type が application/x-www-form-urlencoded または Query パラメータに Action があれば
//    Query プロトコルとして処理します。
// 3. それ以外は REST JSON プロトコル (フォールバック) として処理します。
func (c *AWSCodec) DecodeRequest(r *http.Request) (service.Request, error) {
	if r.Header.Get("X-Amz-Target") != "" {
		return DecodeJSONRequest(r)
	}

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/x-www-form-urlencoded") || r.URL.Query().Get("Action") != "" {
		req, err := DecodeQueryRequest(r)
		if err != nil {
			return service.Request{}, err
		}
		req.Provider = "aws"
		return req, nil
	}

	return DecodeRESTJSONRequest(r)
}

// EncodeResponse は service.Response を HTTP レスポンスとして出力します。
func (c *AWSCodec) EncodeResponse(w http.ResponseWriter, resp service.Response) {
	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}
	if resp.ContentType != "" {
		w.Header().Set("Content-Type", resp.ContentType)
	}
	w.WriteHeader(resp.StatusCode)
	if len(resp.Body) > 0 {
		_, _ = w.Write(resp.Body)
	}
}

// EncodeError はエラーを AWS 互換 XML フォーマットで HTTP レスポンスとして出力します。
func (c *AWSCodec) EncodeError(w http.ResponseWriter, err error, requestID string) {
	code, status := classifyAWSError(err)
	WriteError(w, status, code, err.Error(), requestID)
}

func classifyAWSError(err error) (string, int) {
	switch {
	case errors.Is(err, ErrMissingAction), errors.Is(err, ErrEmptyBody):
		return "MissingAction", http.StatusBadRequest
	default:
		return "InternalError", http.StatusInternalServerError
	}
}
