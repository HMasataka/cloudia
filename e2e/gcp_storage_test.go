//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestE2E_GCS_BasicOperations はGCSの基本的なバケット・オブジェクト操作をテストします。
// gcloudが未インストールの場合はHTTPリクエストで直接テストします。
func TestE2E_GCS_BasicOperations(t *testing.T) {
	skipIfDockerNotAvailable(t)
	// GCSはMinIO(S3バックエンド)に依存するため、利用可能か確認する
	skipIfServiceUnhealthy(t, fmt.Sprintf("%s/storage/v1/b?project=cloudia-local", globalServer.baseURL))

	t.Cleanup(func() { cleanupOrphans(t) })

	const bucketName = "e2e-test-gcs-bucket"
	const projectID = "cloudia-local"

	// バケット作成 (GCS JSON API)
	t.Run("CreateBucket", func(t *testing.T) {
		body := map[string]interface{}{
			"name":         bucketName,
			"location":     "US",
			"storageClass": "STANDARD",
		}
		bodyBytes, _ := json.Marshal(body)

		url := fmt.Sprintf("%s/storage/v1/b?project=%s", globalServer.baseURL, projectID)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(bodyBytes))
		if err != nil {
			t.Fatalf("create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("CreateBucket request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("CreateBucket: expected 200/201, got %d: %s", resp.StatusCode, respBody)
		}
	})

	// バケット一覧
	t.Run("ListBuckets", func(t *testing.T) {
		url := fmt.Sprintf("%s/storage/v1/b?project=%s", globalServer.baseURL, projectID)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("create request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer test-token")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("ListBuckets request: %v", err)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("ListBuckets: expected 200, got %d: %s", resp.StatusCode, respBody)
		}
		if !strings.Contains(string(respBody), bucketName) {
			t.Errorf("ListBuckets: expected %q in response, got: %s", bucketName, respBody)
		}
	})

	// バケット取得
	t.Run("GetBucket", func(t *testing.T) {
		url := fmt.Sprintf("%s/storage/v1/b/%s", globalServer.baseURL, bucketName)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("create request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer test-token")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GetBucket request: %v", err)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GetBucket: expected 200, got %d: %s", resp.StatusCode, respBody)
		}
	})

	// バケット削除
	t.Run("DeleteBucket", func(t *testing.T) {
		url := fmt.Sprintf("%s/storage/v1/b/%s", globalServer.baseURL, bucketName)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, url, nil)
		if err != nil {
			t.Fatalf("create request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer test-token")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("DeleteBucket request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("DeleteBucket: expected 200/204, got %d: %s", resp.StatusCode, respBody)
		}
	})
}
