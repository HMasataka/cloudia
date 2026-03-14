package lambda

import "fmt"

// lambdaRuntimeImages は Lambda ランタイム名から RIE 内蔵イメージへのマッピングです。
// 既存の DefaultRuntimeMap (internal/backend/mapping/runtime.go) とは独立したテーブルです。
var lambdaRuntimeImages = map[string]string{
	"python3.12":      "public.ecr.aws/lambda/python:3.12",
	"python3.11":      "public.ecr.aws/lambda/python:3.11",
	"nodejs20.x":      "public.ecr.aws/lambda/nodejs:20",
	"nodejs18.x":      "public.ecr.aws/lambda/nodejs:18",
	"provided.al2023": "public.ecr.aws/lambda/provided:al2023",
}

// resolveRuntimeImage は Lambda ランタイム名に対応する Docker イメージを返します。
// 未対応のランタイムの場合は error を返します。
func resolveRuntimeImage(runtime string) (string, error) {
	img, ok := lambdaRuntimeImages[runtime]
	if !ok {
		return "", fmt.Errorf("InvalidRuntimeException: unsupported runtime %q", runtime)
	}
	return img, nil
}
