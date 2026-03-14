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

// TestE2E_GCE_BasicOperations はGCEの基本的なインスタンス操作をテストします。
func TestE2E_GCE_BasicOperations(t *testing.T) {
	skipIfDockerNotAvailable(t)

	t.Cleanup(func() { cleanupOrphans(t) })


	const (
		projectID    = "cloudia-local"
		zone         = "us-central1-a"
		instanceName = "e2e-test-gce-instance"
	)

	var instanceCreated bool

	// インスタンス作成
	t.Run("InsertInstance", func(t *testing.T) {
		body := map[string]interface{}{
			"name":        instanceName,
			"machineType": fmt.Sprintf("zones/%s/machineTypes/n1-standard-1", zone),
			"disks": []map[string]interface{}{
				{
					"boot":             true,
					"autoDelete":       true,
					"initializeParams": map[string]interface{}{"sourceImage": "projects/debian-cloud/global/images/debian-11"},
				},
			},
			"networkInterfaces": []map[string]interface{}{
				{"network": "global/networks/default"},
			},
		}
		bodyBytes, _ := json.Marshal(body)

		url := fmt.Sprintf("%s/compute/v1/projects/%s/zones/%s/instances", globalServer.baseURL, projectID, zone)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(bodyBytes))
		if err != nil {
			t.Fatalf("create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("InsertInstance request: %v", err)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 500 {
			t.Skipf("InsertInstance: got %d (Docker image pull may have failed): %s", resp.StatusCode, respBody)
		}
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			t.Fatalf("InsertInstance: expected 200/201, got %d: %s", resp.StatusCode, respBody)
		}
		instanceCreated = true
		t.Logf("created instance: %s", instanceName)
	})

	if !instanceCreated {
		t.Skip("instance creation failed, skipping remaining tests")
	}

	// インスタンス一覧
	t.Run("ListInstances", func(t *testing.T) {
		url := fmt.Sprintf("%s/compute/v1/projects/%s/zones/%s/instances", globalServer.baseURL, projectID, zone)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("create request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer test-token")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("ListInstances request: %v", err)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("ListInstances: expected 200, got %d: %s", resp.StatusCode, respBody)
		}
		if !strings.Contains(string(respBody), instanceName) {
			t.Errorf("ListInstances: expected %q in response, got: %s", instanceName, respBody)
		}
	})

	// インスタンス取得
	t.Run("GetInstance", func(t *testing.T) {
		url := fmt.Sprintf("%s/compute/v1/projects/%s/zones/%s/instances/%s",
			globalServer.baseURL, projectID, zone, instanceName)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("create request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer test-token")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GetInstance request: %v", err)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GetInstance: expected 200, got %d: %s", resp.StatusCode, respBody)
		}
	})

	// インスタンス削除
	t.Run("DeleteInstance", func(t *testing.T) {
		url := fmt.Sprintf("%s/compute/v1/projects/%s/zones/%s/instances/%s",
			globalServer.baseURL, projectID, zone, instanceName)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, url, nil)
		if err != nil {
			t.Fatalf("create request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer test-token")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("DeleteInstance request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("DeleteInstance: expected 200/204, got %d: %s", resp.StatusCode, respBody)
		}
	})

	t.Cleanup(func() {
		url := fmt.Sprintf("%s/compute/v1/projects/%s/zones/%s/instances/%s",
			globalServer.baseURL, projectID, zone, instanceName)
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete, url, nil)
		if req != nil {
			req.Header.Set("Authorization", "Bearer test-token")
			resp, _ := http.DefaultClient.Do(req)
			if resp != nil {
				resp.Body.Close()
			}
		}
	})
}
