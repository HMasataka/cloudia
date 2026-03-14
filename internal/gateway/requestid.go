package gateway

import (
	"context"
	"crypto/rand"
	"fmt"
)

// contextKey はコンテキストキーの型です。
type contextKey string

// RequestIDKey はコンテキストにリクエスト ID を格納するキーです。
const RequestIDKey contextKey = "request_id"

// GenerateRequestID は crypto/rand ベースの UUID v4 形式のリクエスト ID を生成します。
func GenerateRequestID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("gateway: failed to generate request id: %w", err)
	}

	// UUID v4: version bits = 0100, variant bits = 10
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// WithRequestID はコンテキストにリクエスト ID を付与して返します。
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// RequestIDFromContext はコンテキストからリクエスト ID を取得します。
func RequestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(RequestIDKey).(string)
	return v
}
