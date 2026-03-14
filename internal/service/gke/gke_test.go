package gke_test

import (
	"context"
	"encoding/json"
	"testing"

	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/backend/docker"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/service/gke"
	"github.com/HMasataka/cloudia/internal/state"
)

// stubContainerRunner は ContainerRunner のスタブ実装です。
type stubContainerRunner struct{}

func (s *stubContainerRunner) RunContainer(_ context.Context, _ docker.ContainerConfig) (string, error) {
	return "gke-container-stub-id", nil
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
	return docker.ContainerInfo{State: "running", IPAddress: "10.0.0.1"}, nil
}

func (s *stubContainerRunner) ExecInContainer(_ context.Context, _ string, _ []string) ([]byte, error) {
	// k3s kubeconfig の最小限のスタブ
	kubeconfig := `apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: dGVzdC1jYS1kYXRh
    server: https://127.0.0.1:6443
  name: default
contexts:
- context:
    cluster: default
    user: default
  name: default
current-context: default
kind: Config
preferences: {}
users:
- name: default
  user:
    client-certificate-data: dGVzdA==
    client-key-data: dGVzdA==
`
	return []byte(kubeconfig), nil
}

// stubPortAllocator は PortAllocator のスタブ実装です。
type stubPortAllocator struct{}

func (p *stubPortAllocator) Allocate(_ int, _ string) (int, error) { return 16443, nil }
func (p *stubPortAllocator) Release(_ int)                         {}

// stubClusterBackend は ClusterBackend のスタブ実装です。Docker 依存なしでテストできます。
type stubClusterBackend struct{}

func (b *stubClusterBackend) Start(_ context.Context, _ service.ServiceDeps, _ string) error {
	return nil
}
func (b *stubClusterBackend) Kubeconfig() string          { return "stub-kubeconfig" }
func (b *stubClusterBackend) Endpoint() string            { return "https://localhost:16443" }
func (b *stubClusterBackend) CertificateAuthority() string { return "stub-ca-data" }
func (b *stubClusterBackend) ContainerID() string         { return "stub-container-id" }
func (b *stubClusterBackend) Shutdown(_ context.Context) error { return nil }

func stubBackendFactory(_ *zap.Logger) gke.ClusterBackend {
	return &stubClusterBackend{}
}

func newTestGKEService(t *testing.T) (*gke.GKEService, *state.MemoryStore) {
	t.Helper()
	store := state.NewMemoryStore()
	runner := &stubContainerRunner{}
	portAlloc := &stubPortAllocator{}
	svc := gke.NewGKEService(zap.NewNop()).WithBackendFactory(stubBackendFactory)
	if err := svc.Init(context.Background(), service.ServiceDeps{
		Store:         store,
		DockerClient:  runner,
		PortAllocator: portAlloc,
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return svc, store
}

func handleGKERequest(t *testing.T, svc *gke.GKEService, method, action string, body []byte) service.Response {
	t.Helper()
	resp, err := svc.HandleRequest(context.Background(), service.Request{
		Provider: "gcp",
		Service:  "gke",
		Action:   action,
		Method:   method,
		Body:     body,
	})
	if err != nil {
		t.Fatalf("HandleRequest(%s %s): unexpected error: %v", method, action, err)
	}
	return resp
}

const (
	testProject  = "my-project"
	testLocation = "us-central1"
	clustersPath = "projects/my-project/locations/us-central1/clusters"
)

func clusterPath(name string) string {
	return clustersPath + "/" + name
}

// TestGKEService_Name は Name() が "gke" を返すことを検証します。
func TestGKEService_Name(t *testing.T) {
	svc := gke.NewGKEService(zap.NewNop())
	if got := svc.Name(); got != "gke" {
		t.Errorf("Name() = %q, want %q", got, "gke")
	}
}

// TestGKEService_Provider は Provider() が "gcp" を返すことを検証します。
func TestGKEService_Provider(t *testing.T) {
	svc := gke.NewGKEService(zap.NewNop())
	if got := svc.Provider(); got != "gcp" {
		t.Errorf("Provider() = %q, want %q", got, "gcp")
	}
}

// TestGKEService_Lifecycle は Create -> Get -> List -> Delete のライフサイクルを検証します。
func TestGKEService_Lifecycle(t *testing.T) {
	svc, _ := newTestGKEService(t)

	// Create
	createBody, _ := json.Marshal(map[string]interface{}{
		"cluster": map[string]interface{}{
			"name": "test-cluster",
		},
	})
	createResp := handleGKERequest(t, svc, "POST", clustersPath, createBody)
	if createResp.StatusCode != 200 {
		t.Fatalf("createCluster: expected 200, got %d. body=%s", createResp.StatusCode, createResp.Body)
	}

	var op map[string]interface{}
	if err := json.Unmarshal(createResp.Body, &op); err != nil {
		t.Fatalf("createCluster: failed to parse response: %v", err)
	}
	if op["status"] != "DONE" {
		t.Errorf("createCluster: expected status DONE, got %v", op["status"])
	}

	// Get
	getResp := handleGKERequest(t, svc, "GET", clusterPath("test-cluster"), nil)
	if getResp.StatusCode != 200 {
		t.Fatalf("getCluster: expected 200, got %d. body=%s", getResp.StatusCode, getResp.Body)
	}

	var cluster map[string]interface{}
	if err := json.Unmarshal(getResp.Body, &cluster); err != nil {
		t.Fatalf("getCluster: failed to parse response: %v", err)
	}
	if cluster["name"] != "test-cluster" {
		t.Errorf("getCluster: expected name test-cluster, got %v", cluster["name"])
	}
	if cluster["status"] != "RUNNING" {
		t.Errorf("getCluster: expected status RUNNING, got %v", cluster["status"])
	}
	if cluster["endpoint"] == "" || cluster["endpoint"] == nil {
		t.Errorf("getCluster: expected non-empty endpoint, got %v", cluster["endpoint"])
	}
	masterAuth, _ := cluster["masterAuth"].(map[string]interface{})
	if masterAuth == nil || masterAuth["clusterCaCertificate"] == "" {
		t.Errorf("getCluster: expected non-empty masterAuth.clusterCaCertificate, got %v", masterAuth)
	}

	// List
	listResp := handleGKERequest(t, svc, "GET", clustersPath, nil)
	if listResp.StatusCode != 200 {
		t.Fatalf("listClusters: expected 200, got %d. body=%s", listResp.StatusCode, listResp.Body)
	}

	var listResult map[string]interface{}
	if err := json.Unmarshal(listResp.Body, &listResult); err != nil {
		t.Fatalf("listClusters: failed to parse response: %v", err)
	}
	clusters, _ := listResult["clusters"].([]interface{})
	if len(clusters) != 1 {
		t.Errorf("listClusters: expected 1 cluster, got %d", len(clusters))
	}

	// Delete
	deleteResp := handleGKERequest(t, svc, "DELETE", clusterPath("test-cluster"), nil)
	if deleteResp.StatusCode != 200 {
		t.Fatalf("deleteCluster: expected 200, got %d. body=%s", deleteResp.StatusCode, deleteResp.Body)
	}

	// After delete: get should return 404
	getAfterDelete := handleGKERequest(t, svc, "GET", clusterPath("test-cluster"), nil)
	if getAfterDelete.StatusCode != 404 {
		t.Fatalf("getCluster after delete: expected 404, got %d", getAfterDelete.StatusCode)
	}
}

// TestGKEService_CreateDuplicate は同名クラスタの重複作成が 409 を返すことを検証します。
func TestGKEService_CreateDuplicate(t *testing.T) {
	svc, _ := newTestGKEService(t)

	createBody, _ := json.Marshal(map[string]interface{}{
		"cluster": map[string]interface{}{
			"name": "dup-cluster",
		},
	})

	handleGKERequest(t, svc, "POST", clustersPath, createBody)

	resp := handleGKERequest(t, svc, "POST", clustersPath, createBody)
	if resp.StatusCode != 409 {
		t.Fatalf("createCluster duplicate: expected 409, got %d. body=%s", resp.StatusCode, resp.Body)
	}
}

// TestGKEService_GetNotFound は存在しないクラスタの取得が 404 を返すことを検証します。
func TestGKEService_GetNotFound(t *testing.T) {
	svc, _ := newTestGKEService(t)

	resp := handleGKERequest(t, svc, "GET", clusterPath("not-found"), nil)
	if resp.StatusCode != 404 {
		t.Fatalf("getCluster not found: expected 404, got %d. body=%s", resp.StatusCode, resp.Body)
	}
}

// TestGKEService_DeleteNotFound は存在しないクラスタの削除が 404 を返すことを検証します。
func TestGKEService_DeleteNotFound(t *testing.T) {
	svc, _ := newTestGKEService(t)

	resp := handleGKERequest(t, svc, "DELETE", clusterPath("ghost-cluster"), nil)
	if resp.StatusCode != 404 {
		t.Fatalf("deleteCluster not found: expected 404, got %d. body=%s", resp.StatusCode, resp.Body)
	}
}
