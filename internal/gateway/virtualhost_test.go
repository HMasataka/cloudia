package gateway

import (
	"net/http/httptest"
	"testing"
)

func TestExtractVirtualHostBucket(t *testing.T) {
	// Given: バーチャルホスト形式のホスト（ポートあり）
	host := "test-bucket.s3.localhost:4566"

	// When
	bucket, ok := extractVirtualHostBucket(host)

	// Then
	if !ok {
		t.Fatal("expected ok=true, got false")
	}
	if bucket != "test-bucket" {
		t.Errorf("expected bucket=%q, got %q", "test-bucket", bucket)
	}
}

func TestExtractVirtualHostBucket_WithoutPort(t *testing.T) {
	// Given: バーチャルホスト形式のホスト（ポートなし）
	host := "test-bucket.s3.localhost"

	// When
	bucket, ok := extractVirtualHostBucket(host)

	// Then
	if !ok {
		t.Fatal("expected ok=true, got false")
	}
	if bucket != "test-bucket" {
		t.Errorf("expected bucket=%q, got %q", "test-bucket", bucket)
	}
}

func TestExtractVirtualHostBucket_DottedBucket(t *testing.T) {
	// Given: ドット付きバケット名（サポート外）
	host := "my.bucket.s3.localhost:4566"

	// When
	_, ok := extractVirtualHostBucket(host)

	// Then: ドット付きバケット名は false を返す
	if ok {
		t.Error("expected ok=false for dotted bucket name, got true")
	}
}

func TestExtractVirtualHostBucket_NotVirtualHost(t *testing.T) {
	// Given: 通常ホスト（バーチャルホスト形式ではない）
	host := "localhost:4566"

	// When
	_, ok := extractVirtualHostBucket(host)

	// Then
	if ok {
		t.Error("expected ok=false for non-virtual-host, got true")
	}
}

func TestRewriteVirtualHostPath(t *testing.T) {
	// Given: バーチャルホスト形式のリクエスト
	req := httptest.NewRequest("GET", "/object/key", nil)
	req.Host = "test-bucket.s3.localhost:4566"

	// When
	rewriteVirtualHostPath(req)

	// Then: パスが /{bucket}/{original-path} に書き換えられる
	expected := "/test-bucket/object/key"
	if req.URL.Path != expected {
		t.Errorf("expected path=%q, got %q", expected, req.URL.Path)
	}
}
