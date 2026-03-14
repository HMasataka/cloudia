package aws

import (
	"io"
	"net/http"
	"strings"

	"github.com/HMasataka/cloudia/internal/service"
)

// DecodeRESTJSONRequest は REST JSON プロトコルの HTTP リクエストを service.Request に変換します。
//
// URL パスを Action に設定し (先頭スラッシュを除去)、HTTP メソッドを Method に設定します。
// JSON body がある場合は Body に格納します。
// Service フィールドは SigV4 credential scope から Codec 側で設定済みのため、ここでは設定しません。
func DecodeRESTJSONRequest(r *http.Request) (service.Request, error) {
	action := strings.TrimPrefix(r.URL.Path, "/")

	const maxBodySize = 10 * 1024 * 1024 // 10 MB
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
	if err != nil {
		return service.Request{}, err
	}

	headers := map[string]string{
		"Content-Type": r.Header.Get("Content-Type"),
	}

	params := make(map[string]string)
	for k, vals := range r.URL.Query() {
		if len(vals) > 0 {
			params[k] = vals[0]
		}
	}

	return service.Request{
		Provider: "aws",
		Action:   action,
		Method:   r.Method,
		Params:   params,
		Body:     body,
		Headers:  headers,
	}, nil
}
