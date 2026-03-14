package mapping

import (
	"fmt"

	"github.com/HMasataka/cloudia/pkg/models"
)

// DefaultRuntimeMap はランタイム名から実際のランタイム識別子へのデフォルトマッピングです。
var DefaultRuntimeMap = map[string]string{
	"python3.11": "python:3.11-slim",
	"node18":     "node:18-alpine",
	"go1.21":     "golang:1.21-alpine",
}

// ResolveRuntime はランタイム名を実際のランタイム識別子に解決します。
// 未登録のランタイムの場合は models.ErrNotFound をラップしたエラーを返します。
func ResolveRuntime(runtime string) (string, error) {
	return resolveRuntime(runtime, DefaultRuntimeMap)
}

func resolveRuntime(runtime string, m map[string]string) (string, error) {
	v, ok := m[runtime]
	if !ok {
		return "", fmt.Errorf("runtime %q: %w", runtime, models.ErrNotFound)
	}
	return v, nil
}
