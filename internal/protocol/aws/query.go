package aws

import (
	"errors"
	"io"
	"net/http"
	"net/url"

	"github.com/HMasataka/cloudia/internal/service"
)

// ErrMissingAction は Action パラメータが欠落している場合のエラーです。
var ErrMissingAction = errors.New("missing Action parameter")

// ErrEmptyBody はリクエストボディが空の場合のエラーです。
var ErrEmptyBody = errors.New("empty request body")

// DecodeQueryRequest は application/x-www-form-urlencoded 形式の AWS Query プロトコルリクエストを
// service.Request に変換します。
// service.Request の Service フィールドはこの関数では設定しません（Codec が設定します）。
func DecodeQueryRequest(r *http.Request) (service.Request, error) {
	const maxBodySize = 10 * 1024 * 1024 // 10 MB
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
	if err != nil {
		return service.Request{}, err
	}

	if len(body) == 0 {
		return service.Request{}, ErrEmptyBody
	}

	values, err := url.ParseQuery(string(body))
	if err != nil {
		return service.Request{}, err
	}

	action := values.Get("Action")
	if action == "" {
		return service.Request{}, ErrMissingAction
	}

	params := make(map[string]string)
	for key, vals := range values {
		if key == "Action" {
			continue
		}
		if len(vals) > 0 {
			params[key] = vals[0]
		}
	}

	return service.Request{
		Action: action,
		Params: params,
	}, nil
}
