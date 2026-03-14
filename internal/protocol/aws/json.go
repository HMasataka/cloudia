package aws

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/HMasataka/cloudia/internal/service"
)

// DecodeJSONRequest は AWS JSON プロトコルの HTTP リクエストを service.Request に変換します。
//
// X-Amz-Target ヘッダー形式: "<prefix>.<action>"
// 例: "DynamoDB_20120810.PutItem" → service=dynamodb, action=PutItem
//
// Content-Type の application/x-amz-json-1.0 と 1.1 のバージョン差異は
// Headers["Content-Type"] に保持されます。
func DecodeJSONRequest(r *http.Request) (service.Request, error) {
	target := r.Header.Get("X-Amz-Target")
	if target == "" {
		return service.Request{}, fmt.Errorf("aws json: missing X-Amz-Target header")
	}

	dotIdx := strings.LastIndex(target, ".")
	if dotIdx < 0 {
		return service.Request{}, fmt.Errorf("aws json: invalid X-Amz-Target format %q: expected \"<prefix>.<action>\"", target)
	}

	prefix := target[:dotIdx]
	action := target[dotIdx+1:]

	if action == "" {
		return service.Request{}, fmt.Errorf("aws json: empty action in X-Amz-Target %q", target)
	}

	svcName, ok := TargetPrefixToService[prefix]
	if !ok {
		return service.Request{}, fmt.Errorf("aws json: unknown target prefix %q", prefix)
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return service.Request{}, fmt.Errorf("aws json: failed to read request body: %w", err)
	}

	headers := map[string]string{
		"Content-Type": r.Header.Get("Content-Type"),
	}

	return service.Request{
		Provider: "aws",
		Service:  svcName,
		Action:   action,
		Params:   map[string]string{},
		Body:     body,
		Headers:  headers,
	}, nil
}
