package eks_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/HMasataka/cloudia/internal/backend/docker"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/service/eks"
	"github.com/HMasataka/cloudia/internal/state"
	"go.uber.org/zap"
)

// ---- stubs ----

// stubContainerRunner は ContainerRunner のスタブ実装です (Docker 依存なし)。
type stubContainerRunner struct{}

func (s *stubContainerRunner) RunContainer(_ context.Context, _ docker.ContainerConfig) (string, error) {
	return "stub-container-id", nil
}

func (s *stubContainerRunner) StopContainer(_ context.Context, _ string, _ *int) error { return nil }
func (s *stubContainerRunner) RemoveContainer(_ context.Context, _ string) error        { return nil }
func (s *stubContainerRunner) StartContainer(_ context.Context, _ string) error         { return nil }
func (s *stubContainerRunner) PauseContainer(_ context.Context, _ string) error         { return nil }
func (s *stubContainerRunner) UnpauseContainer(_ context.Context, _ string) error       { return nil }

func (s *stubContainerRunner) InspectContainer(_ context.Context, _ string) (docker.ContainerInfo, error) {
	return docker.ContainerInfo{State: "running", IPAddress: "127.0.0.1"}, nil
}

func (s *stubContainerRunner) ExecInContainer(_ context.Context, _ string, _ []string) ([]byte, error) {
	return nil, nil
}

// stubPortAllocator は PortAllocator のスタブ実装です。
type stubPortAllocator struct {
	nextPort int
}

func (p *stubPortAllocator) Allocate(_ int, _ string) (int, error) {
	p.nextPort++
	return 16443 + p.nextPort, nil
}

func (p *stubPortAllocator) Release(_ int) {}

// stubClusterBackend は ClusterBackend のスタブ実装です。
type stubClusterBackend struct {
	endpoint    string
	kubeconfig  string
	caData      string
	containerID string
}

func (b *stubClusterBackend) Start(_ context.Context, _ service.ServiceDeps, _ string) error {
	return nil
}

func (b *stubClusterBackend) Endpoint() string {
	if b.endpoint == "" {
		return "https://localhost:16444"
	}
	return b.endpoint
}

func (b *stubClusterBackend) Kubeconfig() string {
	if b.kubeconfig == "" {
		return "apiVersion: v1\n"
	}
	return b.kubeconfig
}

func (b *stubClusterBackend) CertificateAuthority() string {
	if b.caData == "" {
		return "dGVzdC1jYS1kYXRh"
	}
	return b.caData
}

func (b *stubClusterBackend) ContainerID() string {
	if b.containerID == "" {
		return "stub-container-id"
	}
	return b.containerID
}

func (b *stubClusterBackend) Shutdown(_ context.Context) error { return nil }

// stubBackendFactory はスタブの ClusterBackend を返すファクトリです。
func stubBackendFactory(_ *zap.Logger) eks.ClusterBackend {
	return &stubClusterBackend{}
}

// ---- helpers ----

// newTestEKSService はスタブ ClusterBackend を使う EKSService を生成します。
func newTestEKSService(t *testing.T) *eks.EKSService {
	t.Helper()

	svc := eks.NewEKSService(zap.NewNop())
	svc.SetBackendFactory(stubBackendFactory)

	deps := service.ServiceDeps{
		Store:         state.NewMemoryStore(),
		DockerClient:  &stubContainerRunner{},
		PortAllocator: &stubPortAllocator{},
	}

	if err := svc.Init(context.Background(), deps); err != nil {
		t.Fatalf("Init: %v", err)
	}

	return svc
}

// handleEKSRequest は EKSService.HandleRequest のヘルパーです。
func handleEKSRequest(t *testing.T, svc *eks.EKSService, method, action string, body []byte) service.Response {
	t.Helper()
	resp, err := svc.HandleRequest(context.Background(), service.Request{
		Provider: "aws",
		Service:  "eks",
		Action:   action,
		Method:   method,
		Body:     body,
	})
	if err != nil {
		t.Fatalf("HandleRequest(%s %s): unexpected error: %v", method, action, err)
	}
	return resp
}

// ---- tests ----

// TestEKSService_Name は Name() が "eks" を返すことを検証します。
func TestEKSService_Name(t *testing.T) {
	svc := eks.NewEKSService(zap.NewNop())
	if got := svc.Name(); got != "eks" {
		t.Errorf("Name() = %q, want %q", got, "eks")
	}
}

// TestEKSService_Provider は Provider() が "aws" を返すことを検証します。
func TestEKSService_Provider(t *testing.T) {
	svc := eks.NewEKSService(zap.NewNop())
	if got := svc.Provider(); got != "aws" {
		t.Errorf("Provider() = %q, want %q", got, "aws")
	}
}

// TestEKSService_Lifecycle は Create → Describe → List → Delete のライフサイクルを検証します。
func TestEKSService_Lifecycle(t *testing.T) {
	svc := newTestEKSService(t)

	body, _ := json.Marshal(map[string]string{
		"name":    "test-cluster",
		"version": "1.29",
		"roleArn": "arn:aws:iam::000000000000:role/eks-role",
	})

	// CREATE
	resp := handleEKSRequest(t, svc, http.MethodPost, "clusters", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("createCluster: expected 200, got %d. body=%s", resp.StatusCode, resp.Body)
	}

	var clusterResp struct {
		Cluster struct {
			Name     string `json:"name"`
			Status   string `json:"status"`
			Endpoint string `json:"endpoint"`
			Arn      string `json:"arn"`
		} `json:"cluster"`
	}
	if err := json.Unmarshal(resp.Body, &clusterResp); err != nil {
		t.Fatalf("createCluster: unmarshal response: %v", err)
	}

	if clusterResp.Cluster.Name != "test-cluster" {
		t.Errorf("createCluster: name = %q, want %q", clusterResp.Cluster.Name, "test-cluster")
	}
	if clusterResp.Cluster.Status != "ACTIVE" {
		t.Errorf("createCluster: status = %q, want %q", clusterResp.Cluster.Status, "ACTIVE")
	}
	if clusterResp.Cluster.Arn == "" {
		t.Error("createCluster: arn is empty")
	}

	// DESCRIBE
	descResp := handleEKSRequest(t, svc, http.MethodGet, "clusters/test-cluster", nil)
	if descResp.StatusCode != http.StatusOK {
		t.Fatalf("describeCluster: expected 200, got %d. body=%s", descResp.StatusCode, descResp.Body)
	}

	var descCluster struct {
		Cluster struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"cluster"`
	}
	if err := json.Unmarshal(descResp.Body, &descCluster); err != nil {
		t.Fatalf("describeCluster: unmarshal response: %v", err)
	}
	if descCluster.Cluster.Name != "test-cluster" {
		t.Errorf("describeCluster: name = %q, want %q", descCluster.Cluster.Name, "test-cluster")
	}

	// LIST
	listResp := handleEKSRequest(t, svc, http.MethodGet, "clusters", nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("listClusters: expected 200, got %d. body=%s", listResp.StatusCode, listResp.Body)
	}

	var listResult struct {
		Clusters []string `json:"clusters"`
	}
	if err := json.Unmarshal(listResp.Body, &listResult); err != nil {
		t.Fatalf("listClusters: unmarshal response: %v", err)
	}
	found := false
	for _, name := range listResult.Clusters {
		if name == "test-cluster" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("listClusters: test-cluster not found in %v", listResult.Clusters)
	}

	// DELETE
	delResp := handleEKSRequest(t, svc, http.MethodDelete, "clusters/test-cluster", nil)
	if delResp.StatusCode != http.StatusOK {
		t.Fatalf("deleteCluster: expected 200, got %d. body=%s", delResp.StatusCode, delResp.Body)
	}

	// 削除後に 404
	notFoundResp := handleEKSRequest(t, svc, http.MethodGet, "clusters/test-cluster", nil)
	if notFoundResp.StatusCode != http.StatusNotFound {
		t.Fatalf("describeCluster after delete: expected 404, got %d. body=%s", notFoundResp.StatusCode, notFoundResp.Body)
	}
}

// TestEKSService_CreateCluster_Duplicate は重複クラスタ作成で 409 を返すことを検証します。
func TestEKSService_CreateCluster_Duplicate(t *testing.T) {
	svc := newTestEKSService(t)

	body, _ := json.Marshal(map[string]string{
		"name":    "dup-cluster",
		"version": "1.29",
	})

	// 1回目
	resp1 := handleEKSRequest(t, svc, http.MethodPost, "clusters", body)
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("createCluster (1st): expected 200, got %d. body=%s", resp1.StatusCode, resp1.Body)
	}

	// 2回目 (重複)
	resp2 := handleEKSRequest(t, svc, http.MethodPost, "clusters", body)
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("createCluster duplicate: expected 409, got %d. body=%s", resp2.StatusCode, resp2.Body)
	}

	var errResp struct {
		Type string `json:"__type"`
	}
	if err := json.Unmarshal(resp2.Body, &errResp); err != nil {
		t.Fatalf("createCluster duplicate: unmarshal error response: %v", err)
	}
	if errResp.Type != "ResourceInUseException" {
		t.Errorf("createCluster duplicate: __type = %q, want %q", errResp.Type, "ResourceInUseException")
	}
}

// TestEKSService_DescribeCluster_NotFound は存在しないクラスタの取得で 404 を返すことを検証します。
func TestEKSService_DescribeCluster_NotFound(t *testing.T) {
	svc := eks.NewEKSService(zap.NewNop())
	if err := svc.Init(context.Background(), service.ServiceDeps{
		Store: state.NewMemoryStore(),
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	resp := handleEKSRequest(t, svc, http.MethodGet, "clusters/nonexistent", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("describeCluster not found: expected 404, got %d. body=%s", resp.StatusCode, resp.Body)
	}

	var errResp struct {
		Type string `json:"__type"`
	}
	if err := json.Unmarshal(resp.Body, &errResp); err != nil {
		t.Fatalf("describeCluster not found: unmarshal error response: %v", err)
	}
	if errResp.Type != "ResourceNotFoundException" {
		t.Errorf("describeCluster not found: __type = %q, want %q", errResp.Type, "ResourceNotFoundException")
	}
}

// TestEKSService_ListClusters_Empty は空リストが返ることを検証します。
func TestEKSService_ListClusters_Empty(t *testing.T) {
	svc := eks.NewEKSService(zap.NewNop())
	if err := svc.Init(context.Background(), service.ServiceDeps{
		Store: state.NewMemoryStore(),
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	resp := handleEKSRequest(t, svc, http.MethodGet, "clusters", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("listClusters empty: expected 200, got %d. body=%s", resp.StatusCode, resp.Body)
	}

	var listResult struct {
		Clusters []string `json:"clusters"`
	}
	if err := json.Unmarshal(resp.Body, &listResult); err != nil {
		t.Fatalf("listClusters empty: unmarshal response: %v", err)
	}
	if len(listResult.Clusters) != 0 {
		t.Errorf("listClusters empty: expected 0 clusters, got %d: %v", len(listResult.Clusters), listResult.Clusters)
	}
}
