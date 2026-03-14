package vpc_test

import (
	"context"
	"strings"
	"testing"

	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/service/vpc"
	"github.com/HMasataka/cloudia/internal/state"
	"go.uber.org/zap"
)

// stubNetworkManager は NetworkManager のスタブ実装です。
type stubNetworkManager struct {
	created map[string]string // name → networkID
	removed []string          // removed networkIDs
}

func newStubNetworkManager() *stubNetworkManager {
	return &stubNetworkManager{created: make(map[string]string)}
}

func (s *stubNetworkManager) CreateNetwork(_ context.Context, name, _ string) (string, error) {
	id := "net-" + name
	s.created[name] = id
	return id, nil
}

func (s *stubNetworkManager) RemoveNetwork(_ context.Context, networkID string) error {
	s.removed = append(s.removed, networkID)
	return nil
}

func newTestService(t *testing.T) (*vpc.VPCService, *state.MemoryStore, *stubNetworkManager) {
	t.Helper()
	store := state.NewMemoryStore()
	net := newStubNetworkManager()
	svc := vpc.NewVPCService(config.AWSAuthConfig{}, zap.NewNop())
	if err := svc.Init(context.Background(), service.ServiceDeps{
		Store:          store,
		NetworkManager: net,
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return svc, store, net
}

func handleRequest(t *testing.T, svc *vpc.VPCService, action string, params map[string]string) service.Response {
	t.Helper()
	resp, err := svc.HandleRequest(context.Background(), service.Request{
		Provider: "aws",
		Service:  "vpc",
		Action:   action,
		Params:   params,
	})
	if err != nil {
		t.Fatalf("HandleRequest(%s): unexpected error: %v", action, err)
	}
	return resp
}

// TestCreateVpc_DescribeVpcs は CreateVpc → DescribeVpcs のラウンドトリップを検証します。
func TestCreateVpc_DescribeVpcs(t *testing.T) {
	svc, _, _ := newTestService(t)

	// CreateVpc
	createResp := handleRequest(t, svc, "CreateVpc", map[string]string{
		"CidrBlock": "10.0.0.0/16",
	})
	if createResp.StatusCode != 200 {
		t.Fatalf("CreateVpc: expected 200, got %d. body=%s", createResp.StatusCode, createResp.Body)
	}
	body := string(createResp.Body)
	if !strings.Contains(body, "10.0.0.0/16") {
		t.Errorf("CreateVpc response missing CidrBlock: %s", body)
	}
	if !strings.Contains(body, "vpc-") {
		t.Errorf("CreateVpc response missing vpcId: %s", body)
	}

	// DescribeVpcs (フィルタなし)
	descResp := handleRequest(t, svc, "DescribeVpcs", map[string]string{})
	if descResp.StatusCode != 200 {
		t.Fatalf("DescribeVpcs: expected 200, got %d", descResp.StatusCode)
	}
	descBody := string(descResp.Body)
	if !strings.Contains(descBody, "10.0.0.0/16") {
		t.Errorf("DescribeVpcs response missing CidrBlock: %s", descBody)
	}
}

// TestCreateVpc_DescribeVpcs_Filter は vpc-id フィルタで絞り込みができることを検証します。
func TestCreateVpc_DescribeVpcs_Filter(t *testing.T) {
	svc, _, _ := newTestService(t)

	// 2つの VPC を作成
	handleRequest(t, svc, "CreateVpc", map[string]string{"CidrBlock": "10.0.0.0/16"})
	resp2 := handleRequest(t, svc, "CreateVpc", map[string]string{"CidrBlock": "10.1.0.0/16"})

	// 2つ目の VPC の ID を取得
	body2 := string(resp2.Body)
	vpcIDStart := strings.Index(body2, "<vpcId>")
	vpcIDEnd := strings.Index(body2, "</vpcId>")
	if vpcIDStart < 0 || vpcIDEnd < 0 {
		t.Fatalf("could not extract vpcId from: %s", body2)
	}
	vpcID := body2[vpcIDStart+7 : vpcIDEnd]

	// vpc-id フィルタで絞り込み
	descResp := handleRequest(t, svc, "DescribeVpcs", map[string]string{
		"Filter.1.Name":    "vpc-id",
		"Filter.1.Value.1": vpcID,
	})
	descBody := string(descResp.Body)
	if !strings.Contains(descBody, "10.1.0.0/16") {
		t.Errorf("expected 10.1.0.0/16 in filtered response: %s", descBody)
	}
	if strings.Contains(descBody, "10.0.0.0/16") {
		t.Errorf("unexpected 10.0.0.0/16 in filtered response: %s", descBody)
	}
}

// TestDeleteVpc_WithSubnets_DependencyViolation はサブネットを持つ VPC 削除が DependencyViolation を返すことを検証します。
func TestDeleteVpc_WithSubnets_DependencyViolation(t *testing.T) {
	svc, _, _ := newTestService(t)

	// VPC 作成
	createResp := handleRequest(t, svc, "CreateVpc", map[string]string{"CidrBlock": "10.0.0.0/16"})
	body := string(createResp.Body)
	vpcIDStart := strings.Index(body, "<vpcId>")
	vpcIDEnd := strings.Index(body, "</vpcId>")
	vpcID := body[vpcIDStart+7 : vpcIDEnd]

	// Subnet 作成
	handleRequest(t, svc, "CreateSubnet", map[string]string{
		"VpcId":     vpcID,
		"CidrBlock": "10.0.1.0/24",
	})

	// VPC 削除 → DependencyViolation
	delResp, err := svc.HandleRequest(context.Background(), service.Request{
		Provider: "aws",
		Service:  "vpc",
		Action:   "DeleteVpc",
		Params:   map[string]string{"VpcId": vpcID},
	})
	if err != nil {
		t.Fatalf("DeleteVpc: unexpected error: %v", err)
	}
	if delResp.StatusCode == 200 {
		t.Fatal("DeleteVpc with subnets: expected error, got 200")
	}
	if !strings.Contains(string(delResp.Body), "DependencyViolation") {
		t.Errorf("expected DependencyViolation in response: %s", delResp.Body)
	}
}

// TestCreateSubnet_InvalidVpcID は存在しない VpcId で CreateSubnet が InvalidVpcID.NotFound を返すことを検証します。
func TestCreateSubnet_InvalidVpcID(t *testing.T) {
	svc, _, _ := newTestService(t)

	resp, err := svc.HandleRequest(context.Background(), service.Request{
		Provider: "aws",
		Service:  "vpc",
		Action:   "CreateSubnet",
		Params: map[string]string{
			"VpcId":     "vpc-nonexistent",
			"CidrBlock": "10.0.1.0/24",
		},
	})
	if err != nil {
		t.Fatalf("CreateSubnet: unexpected error: %v", err)
	}
	if resp.StatusCode == 200 {
		t.Fatal("CreateSubnet with invalid VpcId: expected error, got 200")
	}
	if !strings.Contains(string(resp.Body), "InvalidVpcID.NotFound") {
		t.Errorf("expected InvalidVpcID.NotFound in response: %s", resp.Body)
	}
}

// TestShutdown_RemovesNetworks は Shutdown が管理中の Docker ネットワークを全削除することを検証します。
func TestShutdown_RemovesNetworks(t *testing.T) {
	svc, _, net := newTestService(t)

	handleRequest(t, svc, "CreateVpc", map[string]string{"CidrBlock": "10.0.0.0/16"})
	handleRequest(t, svc, "CreateVpc", map[string]string{"CidrBlock": "10.1.0.0/16"})

	if err := svc.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if len(net.removed) != 2 {
		t.Errorf("Shutdown: expected 2 networks removed, got %d", len(net.removed))
	}
}

// TestDescribeSubnets_Filter は subnet-id / vpc-id フィルタで絞り込みができることを検証します。
func TestDescribeSubnets_Filter(t *testing.T) {
	svc, _, _ := newTestService(t)

	// VPC 作成
	createResp := handleRequest(t, svc, "CreateVpc", map[string]string{"CidrBlock": "10.0.0.0/16"})
	body := string(createResp.Body)
	vpcIDStart := strings.Index(body, "<vpcId>")
	vpcIDEnd := strings.Index(body, "</vpcId>")
	vpcID := body[vpcIDStart+7 : vpcIDEnd]

	// Subnet 作成
	subResp := handleRequest(t, svc, "CreateSubnet", map[string]string{
		"VpcId":     vpcID,
		"CidrBlock": "10.0.1.0/24",
	})
	subBody := string(subResp.Body)
	subIDStart := strings.Index(subBody, "<subnetId>")
	subIDEnd := strings.Index(subBody, "</subnetId>")
	subnetID := subBody[subIDStart+10 : subIDEnd]

	// vpc-id フィルタ
	descResp := handleRequest(t, svc, "DescribeSubnets", map[string]string{
		"Filter.1.Name":    "vpc-id",
		"Filter.1.Value.1": vpcID,
	})
	if !strings.Contains(string(descResp.Body), subnetID) {
		t.Errorf("DescribeSubnets vpc-id filter: expected subnetID %s in body: %s", subnetID, descResp.Body)
	}

	// subnet-id フィルタ
	descResp2 := handleRequest(t, svc, "DescribeSubnets", map[string]string{
		"Filter.1.Name":    "subnet-id",
		"Filter.1.Value.1": subnetID,
	})
	if !strings.Contains(string(descResp2.Body), subnetID) {
		t.Errorf("DescribeSubnets subnet-id filter: expected subnetID %s in body: %s", subnetID, descResp2.Body)
	}
}
