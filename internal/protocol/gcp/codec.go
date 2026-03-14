package gcp

import (
	"net/http"

	"github.com/HMasataka/cloudia/internal/service"
)

// GCPCodec は GCP プロバイダ用の protocol.Codec 実装です。
type GCPCodec struct{}

// DecodeRequest は HTTP リクエストを service.Request に変換します。
// URL パスから ResolveGCPService でサービスとリソースパスを解決します。
func (c *GCPCodec) DecodeRequest(r *http.Request) (service.Request, error) {
	svc, resourcePath, err := ResolveGCPService(r.URL.Path)
	if err != nil {
		return service.Request{}, err
	}

	headers := map[string]string{
		"Content-Type": r.Header.Get("Content-Type"),
	}

	return service.Request{
		Provider: "gcp",
		Service:  svc,
		Action:   resourcePath,
		Params:   map[string]string{},
		Headers:  headers,
	}, nil
}

// EncodeResponse は service.Response を HTTP レスポンスとして出力します。
func (c *GCPCodec) EncodeResponse(w http.ResponseWriter, resp service.Response) {
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

// EncodeError はエラーを GCP 互換 JSON フォーマットで HTTP レスポンスとして出力します。
func (c *GCPCodec) EncodeError(w http.ResponseWriter, err error, _ string) {
	WriteError(w, http.StatusBadRequest, err.Error())
}
