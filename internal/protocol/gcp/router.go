package gcp

import (
	"fmt"
	"strings"
)

// servicePrefix はパスプレフィックスとサービス名のマッピングエントリです。
type servicePrefix struct {
	prefix  string
	service string
}

// pathPrefixes は longest prefix match に使用するプレフィックス一覧です。
// 長いプレフィックスを先に配置することで longest match を実現します。
var pathPrefixes = []servicePrefix{
	{prefix: "/upload/storage/v1/", service: "storage"},
	{prefix: "/storage/v1/", service: "storage"},
	{prefix: "/compute/v1/", service: "compute"},
	// /v1/projects/ 配下はパス詳細でサービスを判別する
}

// v1ProjectsServices は /v1/projects/ 配下のパスで使うキーワードとサービスのマッピングです。
var v1ProjectsServices = []struct {
	keyword string
	service string
}{
	{keyword: "/instances/", service: "cloudsql"},
	{keyword: "/clusters/", service: "gke"},
	{keyword: "/instances", service: "memorystore"},
}

// ResolveGCPService は URL パスから GCP サービス名とリソースパスを解決します。
//
// ルール:
//   - /storage/v1/ → service="storage"
//   - /compute/v1/ → service="compute"
//   - /v1/projects/ → パス詳細でサービスを判別 (gke / cloudsql / memorystore)
//
// リソースパスはマッチしたプレフィックスを取り除いた残りのパスです。
func ResolveGCPService(path string) (service string, resourcePath string, err error) {
	// longest prefix match
	best := -1
	for i, entry := range pathPrefixes {
		if strings.HasPrefix(path, entry.prefix) {
			if best < 0 || len(entry.prefix) > len(pathPrefixes[best].prefix) {
				best = i
			}
		}
	}

	if best >= 0 {
		entry := pathPrefixes[best]
		return entry.service, strings.TrimPrefix(path, entry.prefix), nil
	}

	// /v1/projects/ 配下の判別
	const v1ProjectsPrefix = "/v1/projects/"
	if strings.HasPrefix(path, v1ProjectsPrefix) {
		rest := strings.TrimPrefix(path, v1ProjectsPrefix)
		for _, kv := range v1ProjectsServices {
			if idx := strings.Index(rest, kv.keyword); idx >= 0 {
				return kv.service, rest, nil
			}
		}
		return "", "", fmt.Errorf("gcp router: unknown service under /v1/projects/ path %q", path)
	}

	return "", "", fmt.Errorf("gcp router: unknown service path %q", path)
}
