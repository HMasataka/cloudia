package gce_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/HMasataka/cloudia/internal/backend/docker"
	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/service/gce"
	"github.com/HMasataka/cloudia/internal/state"
	"go.uber.org/zap"
)

// stubContainerRunner は ContainerRunner のスタブ実装です。
type stubContainerRunner struct{}

func (s *stubContainerRunner) RunContainer(_ context.Context, _ docker.ContainerConfig) (string, error) {
	return "gce-container-stub-id", nil
}

func (s *stubContainerRunner) StopContainer(_ context.Context, _ string, _ *int) error {
	return nil
}

func (s *stubContainerRunner) RemoveContainer(_ context.Context, _ string) error {
	return nil
}

func (s *stubContainerRunner) StartContainer(_ context.Context, _ string) error {
	return nil
}

func (s *stubContainerRunner) PauseContainer(_ context.Context, _ string) error {
	return nil
}

func (s *stubContainerRunner) UnpauseContainer(_ context.Context, _ string) error {
	return nil
}

func (s *stubContainerRunner) InspectContainer(_ context.Context, _ string) (docker.ContainerInfo, error) {
	return docker.ContainerInfo{State: "running", IPAddress: "10.128.0.2"}, nil
}

func (s *stubContainerRunner) ExecInContainer(_ context.Context, _ string, _ []string) ([]byte, error) {
	return nil, nil
}

func newTestGCEService(t *testing.T) (*gce.GCEService, *state.MemoryStore) {
	t.Helper()
	store := state.NewMemoryStore()
	runner := &stubContainerRunner{}
	svc := gce.NewGCEService(config.GCPAuthConfig{Project: "test-project", Zone: "us-central1-a"}, zap.NewNop())
	if err := svc.Init(context.Background(), service.ServiceDeps{
		Store:        store,
		DockerClient: runner,
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return svc, store
}

func handleGCERequest(t *testing.T, svc *gce.GCEService, method, action string, body []byte) service.Response {
	t.Helper()
	resp, err := svc.HandleRequest(context.Background(), service.Request{
		Provider: "gcp",
		Service:  "compute",
		Action:   action,
		Method:   method,
		Body:     body,
	})
	if err != nil {
		t.Fatalf("HandleRequest(%s %s): unexpected error: %v", method, action, err)
	}
	return resp
}

// TestGCEService_Name は Name() が "compute" を返すことを検証します。
func TestGCEService_Name(t *testing.T) {
	svc := gce.NewGCEService(config.GCPAuthConfig{}, zap.NewNop())
	if got := svc.Name(); got != "compute" {
		t.Errorf("Name() = %q, want %q", got, "compute")
	}
}

// TestGCEService_Provider は Provider() が "gcp" を返すことを検証します。
func TestGCEService_Provider(t *testing.T) {
	svc := gce.NewGCEService(config.GCPAuthConfig{}, zap.NewNop())
	if got := svc.Provider(); got != "gcp" {
		t.Errorf("Provider() = %q, want %q", got, "gcp")
	}
}

// TestGCEService_InsertInstance はインスタンスの作成を検証します。
func TestGCEService_InsertInstance(t *testing.T) {
	svc, _ := newTestGCEService(t)

	body, _ := json.Marshal(map[string]interface{}{
		"name":        "test-instance",
		"machineType": "zones/us-central1-a/machineTypes/e2-micro",
		"disks": []map[string]interface{}{
			{
				"initializeParams": map[string]interface{}{
					"sourceImage": "projects/debian-cloud/global/images/family/debian-11",
				},
			},
		},
	})

	resp := handleGCERequest(t, svc, "POST",
		"projects/my-project/zones/us-central1-a/instances", body)
	if resp.StatusCode != 200 {
		t.Fatalf("InsertInstance: expected 200, got %d. body=%s", resp.StatusCode, resp.Body)
	}

	var op map[string]interface{}
	if err := json.Unmarshal(resp.Body, &op); err != nil {
		t.Fatalf("InsertInstance: failed to parse response: %v", err)
	}
	if op["status"] != "DONE" {
		t.Errorf("InsertInstance: expected status DONE, got %v", op["status"])
	}
}

// TestGCEService_InsertInstance_ShortMachineType は短縮マシンタイプ名が動作することを検証します。
func TestGCEService_InsertInstance_ShortMachineType(t *testing.T) {
	svc, _ := newTestGCEService(t)

	body, _ := json.Marshal(map[string]interface{}{
		"name":        "test-short-type",
		"machineType": "e2-small",
	})

	resp := handleGCERequest(t, svc, "POST",
		"projects/my-project/zones/us-central1-a/instances", body)
	if resp.StatusCode != 200 {
		t.Fatalf("InsertInstance short machineType: expected 200, got %d. body=%s", resp.StatusCode, resp.Body)
	}
}

// TestGCEService_InsertInstance_Duplicate は同名インスタンスの重複作成が 409 を返すことを検証します。
func TestGCEService_InsertInstance_Duplicate(t *testing.T) {
	svc, _ := newTestGCEService(t)

	body, _ := json.Marshal(map[string]interface{}{
		"name":        "dup-instance",
		"machineType": "e2-micro",
	})

	handleGCERequest(t, svc, "POST",
		"projects/my-project/zones/us-central1-a/instances", body)

	resp := handleGCERequest(t, svc, "POST",
		"projects/my-project/zones/us-central1-a/instances", body)
	if resp.StatusCode != 409 {
		t.Fatalf("InsertInstance duplicate: expected 409, got %d. body=%s", resp.StatusCode, resp.Body)
	}
}

// TestGCEService_GetInstance は instances.get が正しく動作することを検証します。
func TestGCEService_GetInstance(t *testing.T) {
	svc, _ := newTestGCEService(t)

	// インスタンスを作成
	body, _ := json.Marshal(map[string]interface{}{
		"name":        "get-test",
		"machineType": "e2-micro",
	})
	handleGCERequest(t, svc, "POST",
		"projects/my-project/zones/us-central1-a/instances", body)

	// get
	resp := handleGCERequest(t, svc, "GET",
		"projects/my-project/zones/us-central1-a/instances/get-test", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("GetInstance: expected 200, got %d. body=%s", resp.StatusCode, resp.Body)
	}

	var item map[string]interface{}
	if err := json.Unmarshal(resp.Body, &item); err != nil {
		t.Fatalf("GetInstance: failed to parse response: %v", err)
	}
	if item["name"] != "get-test" {
		t.Errorf("GetInstance: expected name get-test, got %v", item["name"])
	}
	if item["status"] != "RUNNING" {
		t.Errorf("GetInstance: expected status RUNNING, got %v", item["status"])
	}
}

// TestGCEService_GetInstance_NotFound は存在しないインスタンスの取得が 404 を返すことを検証します。
func TestGCEService_GetInstance_NotFound(t *testing.T) {
	svc, _ := newTestGCEService(t)

	resp := handleGCERequest(t, svc, "GET",
		"projects/my-project/zones/us-central1-a/instances/not-found", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("GetInstance not found: expected 404, got %d. body=%s", resp.StatusCode, resp.Body)
	}
}

// TestGCEService_ListInstances は instances.list が正しく動作することを検証します。
func TestGCEService_ListInstances(t *testing.T) {
	svc, _ := newTestGCEService(t)

	for _, name := range []string{"inst-a", "inst-b"} {
		body, _ := json.Marshal(map[string]interface{}{
			"name":        name,
			"machineType": "e2-micro",
		})
		handleGCERequest(t, svc, "POST",
			"projects/my-project/zones/us-central1-a/instances", body)
	}

	resp := handleGCERequest(t, svc, "GET",
		"projects/my-project/zones/us-central1-a/instances", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("ListInstances: expected 200, got %d. body=%s", resp.StatusCode, resp.Body)
	}

	var listResp map[string]interface{}
	if err := json.Unmarshal(resp.Body, &listResp); err != nil {
		t.Fatalf("ListInstances: failed to parse response: %v", err)
	}

	items, _ := listResp["items"].([]interface{})
	if len(items) != 2 {
		t.Errorf("ListInstances: expected 2 items, got %d", len(items))
	}
}

// TestGCEService_DeleteInstance は instances.delete が正しく動作することを検証します。
func TestGCEService_DeleteInstance(t *testing.T) {
	svc, _ := newTestGCEService(t)

	body, _ := json.Marshal(map[string]interface{}{
		"name":        "to-delete",
		"machineType": "e2-micro",
	})
	handleGCERequest(t, svc, "POST",
		"projects/my-project/zones/us-central1-a/instances", body)

	resp := handleGCERequest(t, svc, "DELETE",
		"projects/my-project/zones/us-central1-a/instances/to-delete", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("DeleteInstance: expected 200, got %d. body=%s", resp.StatusCode, resp.Body)
	}

	// 削除後は get で 404
	getResp := handleGCERequest(t, svc, "GET",
		"projects/my-project/zones/us-central1-a/instances/to-delete", nil)
	if getResp.StatusCode != 404 {
		t.Fatalf("GetInstance after delete: expected 404, got %d", getResp.StatusCode)
	}
}

// TestGCEService_StopInstance は instances.stop が正しく動作することを検証します。
func TestGCEService_StopInstance(t *testing.T) {
	svc, _ := newTestGCEService(t)

	body, _ := json.Marshal(map[string]interface{}{
		"name":        "stop-test",
		"machineType": "e2-micro",
	})
	handleGCERequest(t, svc, "POST",
		"projects/my-project/zones/us-central1-a/instances", body)

	resp := handleGCERequest(t, svc, "POST",
		"projects/my-project/zones/us-central1-a/instances/stop-test/stop", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("StopInstance: expected 200, got %d. body=%s", resp.StatusCode, resp.Body)
	}

	// get でステータス確認
	getResp := handleGCERequest(t, svc, "GET",
		"projects/my-project/zones/us-central1-a/instances/stop-test", nil)
	var item map[string]interface{}
	if err := json.Unmarshal(getResp.Body, &item); err != nil {
		t.Fatalf("GetInstance after stop: failed to parse response: %v", err)
	}
	if item["status"] != "TERMINATED" {
		t.Errorf("StopInstance: expected status TERMINATED, got %v", item["status"])
	}
}

// TestGCEService_StartInstance は instances.start が正しく動作することを検証します。
func TestGCEService_StartInstance(t *testing.T) {
	svc, _ := newTestGCEService(t)

	body, _ := json.Marshal(map[string]interface{}{
		"name":        "start-test",
		"machineType": "e2-micro",
	})
	handleGCERequest(t, svc, "POST",
		"projects/my-project/zones/us-central1-a/instances", body)

	// まず停止
	handleGCERequest(t, svc, "POST",
		"projects/my-project/zones/us-central1-a/instances/start-test/stop", nil)

	// 起動
	resp := handleGCERequest(t, svc, "POST",
		"projects/my-project/zones/us-central1-a/instances/start-test/start", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("StartInstance: expected 200, got %d. body=%s", resp.StatusCode, resp.Body)
	}

	// get でステータス確認
	getResp := handleGCERequest(t, svc, "GET",
		"projects/my-project/zones/us-central1-a/instances/start-test", nil)
	var item map[string]interface{}
	if err := json.Unmarshal(getResp.Body, &item); err != nil {
		t.Fatalf("GetInstance after start: failed to parse response: %v", err)
	}
	if item["status"] != "RUNNING" {
		t.Errorf("StartInstance: expected status RUNNING, got %v", item["status"])
	}
}
