package mapping

import "strings"

const defaultGCEDockerImage = "ubuntu:22.04"

// defaultGCEImageMap は GCE イメージファミリー/ソースイメージパターンから Docker イメージへのマッピングです。
var defaultGCEImageMap = map[string]string{
	"debian-11":        "debian:11",
	"ubuntu-2204-lts":  "ubuntu:22.04",
	"centos-stream-9":  "centos:stream9",
}

// ResolveGCEDockerImage は GCE sourceImage パスまたはイメージファミリー名を Docker イメージ名に変換します。
// 例: "projects/debian-cloud/global/images/family/debian-11" -> "debian:11"
// 例: "debian-11" -> "debian:11"
func ResolveGCEDockerImage(sourceImage string) string {
	return resolveGCEDockerImage(sourceImage, defaultGCEImageMap)
}

func resolveGCEDockerImage(sourceImage string, m map[string]string) string {
	if sourceImage == "" {
		return defaultGCEDockerImage
	}

	// フルパス形式: .../family/{family} or .../images/{image-name}
	// 末尾のセグメントを取得する
	parts := strings.Split(sourceImage, "/")
	last := parts[len(parts)-1]

	if v, ok := m[last]; ok {
		return v
	}

	// キー全体でも検索
	if v, ok := m[sourceImage]; ok {
		return v
	}

	return defaultGCEDockerImage
}
