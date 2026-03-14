package memorystore

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/state"
	"go.uber.org/zap"
)

// newTestMemorystoreService は Registry/Redis 依存なしでサービスを構築します。
// store を直接注入し、redisHost/redisPort はダミー値を使います。
func newTestMemorystoreService(t *testing.T) (*MemorystoreService, *state.MemoryStore) {
	t.Helper()
	store := state.NewMemoryStore()
	svc := &MemorystoreService{
		store:     store,
		logger:    zap.NewNop(),
		redisHost: "localhost",
		redisPort: "6379",
	}
	return svc, store
}

func handleMemorystoreRequest(t *testing.T, svc *MemorystoreService, method, action string, body []byte) service.Response {
	t.Helper()
	resp, err := svc.HandleRequest(context.Background(), service.Request{
		Provider: "gcp",
		Service:  "memorystore",
		Action:   action,
		Method:   method,
		Body:     body,
	})
	if err != nil {
		t.Fatalf("HandleRequest(%s %s): unexpected error: %v", method, action, err)
	}
	return resp
}

// TestMemorystoreService_Name は Name() が "memorystore" を返すことを検証します。
func TestMemorystoreService_Name(t *testing.T) {
	svc := NewMemorystoreService(zap.NewNop())
	if got := svc.Name(); got != "memorystore" {
		t.Errorf("Name() = %q, want %q", got, "memorystore")
	}
}

// TestMemorystoreService_Provider は Provider() が "gcp" を返すことを検証します。
func TestMemorystoreService_Provider(t *testing.T) {
	svc := NewMemorystoreService(zap.NewNop())
	if got := svc.Provider(); got != "gcp" {
		t.Errorf("Provider() = %q, want %q", got, "gcp")
	}
}

// TestMemorystoreService_Create_Get_List_Delete は作成 → Get → List → Delete の正常系を検証します。
func TestMemorystoreService_Create_Get_List_Delete(t *testing.T) {
	svc, _ := newTestMemorystoreService(t)

	project := "my-project"
	location := "us-central1"
	instanceName := "my-redis"

	// Create
	createBody, _ := json.Marshal(map[string]interface{}{
		"name":         instanceName,
		"tier":         "BASIC",
		"memorySizeGb": 1,
		"redisVersion": "REDIS_7_0",
	})
	createResp := handleMemorystoreRequest(t, svc,
		http.MethodPost,
		"projects/"+project+"/locations/"+location+"/instances",
		createBody,
	)
	if createResp.StatusCode != 200 {
		t.Fatalf("Create: expected 200, got %d. body=%s", createResp.StatusCode, createResp.Body)
	}
	createBodyStr := string(createResp.Body)
	if !strings.Contains(createBodyStr, instanceName) {
		t.Errorf("Create: response missing instance name: %s", createBodyStr)
	}

	// Get
	getResp := handleMemorystoreRequest(t, svc,
		http.MethodGet,
		"projects/"+project+"/locations/"+location+"/instances/"+instanceName,
		nil,
	)
	if getResp.StatusCode != 200 {
		t.Fatalf("Get: expected 200, got %d. body=%s", getResp.StatusCode, getResp.Body)
	}
	getBodyStr := string(getResp.Body)
	if !strings.Contains(getBodyStr, instanceName) {
		t.Errorf("Get: instance name not found in response: %s", getBodyStr)
	}
	if !strings.Contains(getBodyStr, "READY") {
		t.Errorf("Get: expected READY state: %s", getBodyStr)
	}

	// List
	listResp := handleMemorystoreRequest(t, svc,
		http.MethodGet,
		"projects/"+project+"/locations/"+location+"/instances",
		nil,
	)
	if listResp.StatusCode != 200 {
		t.Fatalf("List: expected 200, got %d. body=%s", listResp.StatusCode, listResp.Body)
	}
	listBodyStr := string(listResp.Body)
	if !strings.Contains(listBodyStr, instanceName) {
		t.Errorf("List: instance name not found in response: %s", listBodyStr)
	}

	// Delete
	delResp := handleMemorystoreRequest(t, svc,
		http.MethodDelete,
		"projects/"+project+"/locations/"+location+"/instances/"+instanceName,
		nil,
	)
	if delResp.StatusCode != 200 {
		t.Fatalf("Delete: expected 200, got %d. body=%s", delResp.StatusCode, delResp.Body)
	}

	// 削除後は Get でエラーになることを確認
	getAfterDelResp := handleMemorystoreRequest(t, svc,
		http.MethodGet,
		"projects/"+project+"/locations/"+location+"/instances/"+instanceName,
		nil,
	)
	if getAfterDelResp.StatusCode == 200 {
		t.Errorf("Get after delete: expected error, got 200: %s", getAfterDelResp.Body)
	}
	if !strings.Contains(string(getAfterDelResp.Body), "NOT_FOUND") {
		t.Errorf("Get after delete: expected NOT_FOUND: %s", getAfterDelResp.Body)
	}
}

// TestMemorystoreService_Get_NotFound は存在しないインスタンスの Get で NOT_FOUND を返すことを検証します。
func TestMemorystoreService_Get_NotFound(t *testing.T) {
	svc, _ := newTestMemorystoreService(t)

	resp := handleMemorystoreRequest(t, svc,
		http.MethodGet,
		"projects/my-project/locations/us-central1/instances/nonexistent",
		nil,
	)
	if resp.StatusCode == 200 {
		t.Fatal("Get nonexistent instance: expected error, got 200")
	}
	if !strings.Contains(string(resp.Body), "NOT_FOUND") {
		t.Errorf("Get nonexistent instance: expected NOT_FOUND: %s", resp.Body)
	}
}

// TestMemorystoreService_Delete_NotFound は存在しないインスタンスの Delete で NOT_FOUND を返すことを検証します。
func TestMemorystoreService_Delete_NotFound(t *testing.T) {
	svc, _ := newTestMemorystoreService(t)

	resp := handleMemorystoreRequest(t, svc,
		http.MethodDelete,
		"projects/my-project/locations/us-central1/instances/nonexistent",
		nil,
	)
	if resp.StatusCode == 200 {
		t.Fatal("Delete nonexistent instance: expected error, got 200")
	}
	if !strings.Contains(string(resp.Body), "NOT_FOUND") {
		t.Errorf("Delete nonexistent instance: expected NOT_FOUND: %s", resp.Body)
	}
}

// TestMemorystoreService_InvalidPath は不正なパスで INVALID_ARGUMENT を返すことを検証します。
func TestMemorystoreService_InvalidPath(t *testing.T) {
	svc, _ := newTestMemorystoreService(t)

	resp := handleMemorystoreRequest(t, svc,
		http.MethodGet,
		"invalid/path",
		nil,
	)
	if resp.StatusCode == 200 {
		t.Fatal("Invalid path: expected error, got 200")
	}
	if !strings.Contains(string(resp.Body), "INVALID_ARGUMENT") {
		t.Errorf("Invalid path: expected INVALID_ARGUMENT: %s", resp.Body)
	}
}

// TestMemorystoreService_List_Empty は空のリストが返ることを検証します。
func TestMemorystoreService_List_Empty(t *testing.T) {
	svc, _ := newTestMemorystoreService(t)

	resp := handleMemorystoreRequest(t, svc,
		http.MethodGet,
		"projects/my-project/locations/us-central1/instances",
		nil,
	)
	if resp.StatusCode != 200 {
		t.Fatalf("List empty: expected 200, got %d. body=%s", resp.StatusCode, resp.Body)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		t.Fatalf("List empty: failed to parse JSON: %v", err)
	}
	// instances キーが nil または空スライスであることを確認
	instances, _ := result["instances"]
	if instances != nil {
		t.Errorf("List empty: expected no instances, got: %v", instances)
	}
}
