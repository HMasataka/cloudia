package protocol

import (
	"net/http"

	"github.com/HMasataka/cloudia/internal/service"
)

// Codec はHTTPリクエスト/レスポンスとサービス層の型を相互変換するインターフェースです。
// プロバイダごとに異なる実装を持ちます。
type Codec interface {
	// DecodeRequest はHTTPリクエストを正規化されたservice.Requestに変換します。
	DecodeRequest(*http.Request) (service.Request, error)

	// EncodeResponse はservice.ResponseをHTTPレスポンスとして出力します。
	EncodeResponse(http.ResponseWriter, service.Response)

	// EncodeError はエラーをプロバイダ互換フォーマットでHTTPレスポンスとして出力します。
	// 第3引数requestIDはエラーレスポンスに含めるリクエストIDです。
	EncodeError(http.ResponseWriter, error, string)
}
