package sg_test

import (
	"context"
	"strings"
	"testing"

	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/service/sg"
	"github.com/HMasataka/cloudia/internal/state"
	"go.uber.org/zap"
)

func newTestService(t *testing.T) (*sg.SGService, *state.MemoryStore) {
	t.Helper()
	store := state.NewMemoryStore()
	svc := sg.NewSGService(config.AWSAuthConfig{}, zap.NewNop())
	if err := svc.Init(context.Background(), service.ServiceDeps{
		Store: store,
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return svc, store
}

func handleRequest(t *testing.T, svc *sg.SGService, action string, params map[string]string) service.Response {
	t.Helper()
	resp, err := svc.HandleRequest(context.Background(), service.Request{
		Provider: "aws",
		Service:  "sg",
		Action:   action,
		Params:   params,
	})
	if err != nil {
		t.Fatalf("HandleRequest(%s): unexpected error: %v", action, err)
	}
	return resp
}

func extractGroupId(t *testing.T, body string) string {
	t.Helper()
	start := strings.Index(body, "<groupId>")
	end := strings.Index(body, "</groupId>")
	if start < 0 || end < 0 {
		t.Fatalf("could not extract groupId from: %s", body)
	}
	return body[start+9 : end]
}

// TestCreateSecurityGroup_Describe は CreateSecurityGroup → DescribeSecurityGroups のラウンドトリップを検証します。
func TestCreateSecurityGroup_Describe(t *testing.T) {
	svc, _ := newTestService(t)

	createResp := handleRequest(t, svc, "CreateSecurityGroup", map[string]string{
		"GroupName":   "test-sg",
		"Description": "Test security group",
		"VpcId":       "vpc-abc123",
	})
	if createResp.StatusCode != 200 {
		t.Fatalf("CreateSecurityGroup: expected 200, got %d. body=%s", createResp.StatusCode, createResp.Body)
	}
	body := string(createResp.Body)
	if !strings.Contains(body, "sg-") {
		t.Errorf("CreateSecurityGroup response missing groupId: %s", body)
	}

	// DescribeSecurityGroups (フィルタなし)
	descResp := handleRequest(t, svc, "DescribeSecurityGroups", map[string]string{})
	if descResp.StatusCode != 200 {
		t.Fatalf("DescribeSecurityGroups: expected 200, got %d", descResp.StatusCode)
	}
	descBody := string(descResp.Body)
	if !strings.Contains(descBody, "test-sg") {
		t.Errorf("DescribeSecurityGroups response missing GroupName: %s", descBody)
	}
}

// TestCreateSecurityGroup_Duplicate は同名のセキュリティグループ作成が InvalidGroup.Duplicate を返すことを検証します。
func TestCreateSecurityGroup_Duplicate(t *testing.T) {
	svc, _ := newTestService(t)

	handleRequest(t, svc, "CreateSecurityGroup", map[string]string{
		"GroupName":   "duplicate-sg",
		"Description": "First",
	})

	resp, err := svc.HandleRequest(context.Background(), service.Request{
		Provider: "aws",
		Service:  "sg",
		Action:   "CreateSecurityGroup",
		Params: map[string]string{
			"GroupName":   "duplicate-sg",
			"Description": "Second",
		},
	})
	if err != nil {
		t.Fatalf("CreateSecurityGroup duplicate: unexpected error: %v", err)
	}
	if resp.StatusCode == 200 {
		t.Fatal("CreateSecurityGroup duplicate: expected error, got 200")
	}
	if !strings.Contains(string(resp.Body), "InvalidGroup.Duplicate") {
		t.Errorf("expected InvalidGroup.Duplicate in response: %s", resp.Body)
	}
}

// TestCreateSecurityGroup_DefaultEgressRule は CreateSecurityGroup がデフォルト egress ルールを自動追加することを検証します。
func TestCreateSecurityGroup_DefaultEgressRule(t *testing.T) {
	svc, _ := newTestService(t)

	createResp := handleRequest(t, svc, "CreateSecurityGroup", map[string]string{
		"GroupName":   "egress-sg",
		"Description": "Egress test",
	})
	groupID := extractGroupId(t, string(createResp.Body))

	descResp := handleRequest(t, svc, "DescribeSecurityGroups", map[string]string{
		"GroupId.1": groupID,
	})
	body := string(descResp.Body)
	if !strings.Contains(body, "0.0.0.0/0") {
		t.Errorf("expected default egress rule 0.0.0.0/0 in response: %s", body)
	}
}

// TestDeleteSecurityGroup は DeleteSecurityGroup の基本動作を検証します。
func TestDeleteSecurityGroup(t *testing.T) {
	svc, _ := newTestService(t)

	createResp := handleRequest(t, svc, "CreateSecurityGroup", map[string]string{
		"GroupName":   "delete-sg",
		"Description": "To be deleted",
	})
	groupID := extractGroupId(t, string(createResp.Body))

	delResp := handleRequest(t, svc, "DeleteSecurityGroup", map[string]string{
		"GroupId": groupID,
	})
	if delResp.StatusCode != 200 {
		t.Fatalf("DeleteSecurityGroup: expected 200, got %d. body=%s", delResp.StatusCode, delResp.Body)
	}
	if !strings.Contains(string(delResp.Body), "<return>true</return>") {
		t.Errorf("DeleteSecurityGroup: expected return=true in response: %s", delResp.Body)
	}

	// 再度 Describe して存在しないことを確認
	descResp := handleRequest(t, svc, "DescribeSecurityGroups", map[string]string{})
	if strings.Contains(string(descResp.Body), "delete-sg") {
		t.Errorf("DeleteSecurityGroup: group still present after deletion: %s", descResp.Body)
	}
}

// TestDeleteSecurityGroup_NotFound は存在しない SG の削除が InvalidGroup.NotFound を返すことを検証します。
func TestDeleteSecurityGroup_NotFound(t *testing.T) {
	svc, _ := newTestService(t)

	resp, err := svc.HandleRequest(context.Background(), service.Request{
		Provider: "aws",
		Service:  "sg",
		Action:   "DeleteSecurityGroup",
		Params:   map[string]string{"GroupId": "sg-nonexistent"},
	})
	if err != nil {
		t.Fatalf("DeleteSecurityGroup: unexpected error: %v", err)
	}
	if resp.StatusCode == 200 {
		t.Fatal("DeleteSecurityGroup nonexistent: expected error, got 200")
	}
	if !strings.Contains(string(resp.Body), "InvalidGroup.NotFound") {
		t.Errorf("expected InvalidGroup.NotFound in response: %s", resp.Body)
	}
}

// TestAuthorizeSecurityGroupIngress は AuthorizeSecurityGroupIngress の基本動作を検証します。
func TestAuthorizeSecurityGroupIngress(t *testing.T) {
	svc, _ := newTestService(t)

	createResp := handleRequest(t, svc, "CreateSecurityGroup", map[string]string{
		"GroupName":   "ingress-sg",
		"Description": "Ingress test",
	})
	groupID := extractGroupId(t, string(createResp.Body))

	authResp := handleRequest(t, svc, "AuthorizeSecurityGroupIngress", map[string]string{
		"GroupId":                              groupID,
		"IpPermissions.1.IpProtocol":          "tcp",
		"IpPermissions.1.FromPort":            "80",
		"IpPermissions.1.ToPort":              "80",
		"IpPermissions.1.IpRanges.1.CidrIp":  "0.0.0.0/0",
	})
	if authResp.StatusCode != 200 {
		t.Fatalf("AuthorizeSecurityGroupIngress: expected 200, got %d. body=%s", authResp.StatusCode, authResp.Body)
	}

	descResp := handleRequest(t, svc, "DescribeSecurityGroups", map[string]string{
		"GroupId.1": groupID,
	})
	body := string(descResp.Body)
	if !strings.Contains(body, "tcp") {
		t.Errorf("expected tcp protocol in ingress rules: %s", body)
	}
}

// TestRevokeSecurityGroupIngress は RevokeSecurityGroupIngress の基本動作を検証します。
func TestRevokeSecurityGroupIngress(t *testing.T) {
	svc, _ := newTestService(t)

	createResp := handleRequest(t, svc, "CreateSecurityGroup", map[string]string{
		"GroupName":   "revoke-sg",
		"Description": "Revoke test",
	})
	groupID := extractGroupId(t, string(createResp.Body))

	handleRequest(t, svc, "AuthorizeSecurityGroupIngress", map[string]string{
		"GroupId":                             groupID,
		"IpPermissions.1.IpProtocol":         "tcp",
		"IpPermissions.1.FromPort":           "443",
		"IpPermissions.1.ToPort":             "443",
		"IpPermissions.1.IpRanges.1.CidrIp": "0.0.0.0/0",
	})

	revokeResp := handleRequest(t, svc, "RevokeSecurityGroupIngress", map[string]string{
		"GroupId":                             groupID,
		"IpPermissions.1.IpProtocol":         "tcp",
		"IpPermissions.1.FromPort":           "443",
		"IpPermissions.1.ToPort":             "443",
		"IpPermissions.1.IpRanges.1.CidrIp": "0.0.0.0/0",
	})
	if revokeResp.StatusCode != 200 {
		t.Fatalf("RevokeSecurityGroupIngress: expected 200, got %d. body=%s", revokeResp.StatusCode, revokeResp.Body)
	}
	if !strings.Contains(string(revokeResp.Body), "<return>true</return>") {
		t.Errorf("RevokeSecurityGroupIngress: expected return=true: %s", revokeResp.Body)
	}

	// ルールが実際に削除されていることを確認
	descResp := handleRequest(t, svc, "DescribeSecurityGroups", map[string]string{
		"GroupId.1": groupID,
	})
	body := string(descResp.Body)
	if strings.Contains(body, ">443<") {
		t.Errorf("RevokeSecurityGroupIngress: rule still present after revoke: %s", body)
	}
}

// TestRevokeSecurityGroupIngress_RuleRemoved は Revoke 後にルールが削除され、他のルールが残ることを検証します。
func TestRevokeSecurityGroupIngress_RuleRemoved(t *testing.T) {
	svc, _ := newTestService(t)

	createResp := handleRequest(t, svc, "CreateSecurityGroup", map[string]string{
		"GroupName":   "revoke-partial-sg",
		"Description": "Revoke partial test",
	})
	groupID := extractGroupId(t, string(createResp.Body))

	// ルール1 (tcp/80) と ルール2 (tcp/443) を追加
	handleRequest(t, svc, "AuthorizeSecurityGroupIngress", map[string]string{
		"GroupId":                             groupID,
		"IpPermissions.1.IpProtocol":         "tcp",
		"IpPermissions.1.FromPort":           "80",
		"IpPermissions.1.ToPort":             "80",
		"IpPermissions.1.IpRanges.1.CidrIp": "0.0.0.0/0",
	})
	handleRequest(t, svc, "AuthorizeSecurityGroupIngress", map[string]string{
		"GroupId":                             groupID,
		"IpPermissions.1.IpProtocol":         "tcp",
		"IpPermissions.1.FromPort":           "443",
		"IpPermissions.1.ToPort":             "443",
		"IpPermissions.1.IpRanges.1.CidrIp": "0.0.0.0/0",
	})

	// ルール1 (tcp/80) のみ削除
	revokeResp := handleRequest(t, svc, "RevokeSecurityGroupIngress", map[string]string{
		"GroupId":                             groupID,
		"IpPermissions.1.IpProtocol":         "tcp",
		"IpPermissions.1.FromPort":           "80",
		"IpPermissions.1.ToPort":             "80",
		"IpPermissions.1.IpRanges.1.CidrIp": "0.0.0.0/0",
	})
	if revokeResp.StatusCode != 200 {
		t.Fatalf("RevokeSecurityGroupIngress: expected 200, got %d. body=%s", revokeResp.StatusCode, revokeResp.Body)
	}

	// tcp/80 が削除され、tcp/443 が残っていることを確認
	descResp := handleRequest(t, svc, "DescribeSecurityGroups", map[string]string{
		"GroupId.1": groupID,
	})
	body := string(descResp.Body)
	if strings.Contains(body, ">80<") {
		t.Errorf("RevokeSecurityGroupIngress: tcp/80 rule still present after revoke: %s", body)
	}
	if !strings.Contains(body, ">443<") {
		t.Errorf("RevokeSecurityGroupIngress: tcp/443 rule missing after revoke of tcp/80: %s", body)
	}
}

// TestRevokeSecurityGroupEgress_RuleRemoved は RevokeSecurityGroupEgress 後にルールが削除されることを検証します。
func TestRevokeSecurityGroupEgress_RuleRemoved(t *testing.T) {
	svc, _ := newTestService(t)

	createResp := handleRequest(t, svc, "CreateSecurityGroup", map[string]string{
		"GroupName":   "revoke-egress-sg",
		"Description": "Revoke egress test",
	})
	groupID := extractGroupId(t, string(createResp.Body))

	// カスタム egress ルール (tcp/8080) を追加
	handleRequest(t, svc, "AuthorizeSecurityGroupEgress", map[string]string{
		"GroupId":                             groupID,
		"IpPermissions.1.IpProtocol":         "tcp",
		"IpPermissions.1.FromPort":           "8080",
		"IpPermissions.1.ToPort":             "8080",
		"IpPermissions.1.IpRanges.1.CidrIp": "10.0.0.0/8",
	})

	// 追加されていることを確認
	descBefore := handleRequest(t, svc, "DescribeSecurityGroups", map[string]string{
		"GroupId.1": groupID,
	})
	if !strings.Contains(string(descBefore.Body), ">8080<") {
		t.Fatalf("expected egress rule 8080 to be present: %s", descBefore.Body)
	}

	// egress ルールを削除
	revokeResp := handleRequest(t, svc, "RevokeSecurityGroupEgress", map[string]string{
		"GroupId":                             groupID,
		"IpPermissions.1.IpProtocol":         "tcp",
		"IpPermissions.1.FromPort":           "8080",
		"IpPermissions.1.ToPort":             "8080",
		"IpPermissions.1.IpRanges.1.CidrIp": "10.0.0.0/8",
	})
	if revokeResp.StatusCode != 200 {
		t.Fatalf("RevokeSecurityGroupEgress: expected 200, got %d. body=%s", revokeResp.StatusCode, revokeResp.Body)
	}

	// 削除されていることを確認
	descAfter := handleRequest(t, svc, "DescribeSecurityGroups", map[string]string{
		"GroupId.1": groupID,
	})
	if strings.Contains(string(descAfter.Body), ">8080<") {
		t.Errorf("RevokeSecurityGroupEgress: egress rule 8080 still present after revoke: %s", descAfter.Body)
	}
}
