package aws

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/HMasataka/cloudia/internal/service"
)

// AWSCodec は AWS プロバイダ用の protocol.Codec 実装です。
type AWSCodec struct{}

// DecodeRequest は HTTP リクエストを service.Request に変換します。
// X-Amz-Target ヘッダーがあれば JSON プロトコル、Content-Type が
// application/x-www-form-urlencoded であれば Query プロトコルとして処理します。
func (c *AWSCodec) DecodeRequest(r *http.Request) (service.Request, error) {
	if r.Header.Get("X-Amz-Target") != "" {
		return DecodeJSONRequest(r)
	}

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
		req, err := DecodeQueryRequest(r)
		if err != nil {
			return service.Request{}, err
		}
		req.Provider = "aws"
		return req, nil
	}

	return service.Request{}, fmt.Errorf("aws codec: unsupported Content-Type %q: expected X-Amz-Target header or application/x-www-form-urlencoded", ct)
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
