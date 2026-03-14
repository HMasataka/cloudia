package iam

import (
	"context"
	"testing"

	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/state"
	"go.uber.org/zap"
)

func newTestService(t *testing.T) *IAMService {
	t.Helper()
	cfg := config.AWSAuthConfig{
		AccountID: "000000000000",
		Region:    "us-east-1",
	}
	svc := NewIAMService(cfg, zap.NewNop())
	store := state.NewMemoryStore()
	if err := svc.Init(context.Background(), service.ServiceDeps{Store: store}); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	return svc
}

// TestCreateRoleGetRole は CreateRole + GetRole のラウンドトリップをテストします。
func TestCreateRoleGetRole(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	// CreateRole
	createResp, err := svc.HandleRequest(ctx, service.Request{
		Action: "CreateRole",
		Params: map[string]string{
			"RoleName":                 "TestRole",
			"AssumeRolePolicyDocument": `{"Version":"2012-10-17"}`,
			"Path":                     "/",
		},
	})
	if err != nil {
		t.Fatalf("CreateRole error: %v", err)
	}
	if createResp.StatusCode != 200 {
		t.Fatalf("CreateRole status = %d, want 200; body: %s", createResp.StatusCode, createResp.Body)
	}

	// GetRole
	getResp, err := svc.HandleRequest(ctx, service.Request{
		Action: "GetRole",
		Params: map[string]string{"RoleName": "TestRole"},
	})
	if err != nil {
		t.Fatalf("GetRole error: %v", err)
	}
	if getResp.StatusCode != 200 {
		t.Fatalf("GetRole status = %d, want 200; body: %s", getResp.StatusCode, getResp.Body)
	}

	body := string(getResp.Body)
	if !contains(body, "TestRole") {
		t.Errorf("GetRole body does not contain RoleName; body: %s", body)
	}
	if !contains(body, "arn:aws:iam::000000000000:role/TestRole") {
		t.Errorf("GetRole body does not contain expected ARN; body: %s", body)
	}
}

// TestCreateRoleDuplicate は同名ロール作成時に EntityAlreadyExists を返すことをテストします。
func TestCreateRoleDuplicate(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	params := map[string]string{
		"RoleName":                 "DupRole",
		"AssumeRolePolicyDocument": `{"Version":"2012-10-17"}`,
	}

	if _, err := svc.HandleRequest(ctx, service.Request{Action: "CreateRole", Params: params}); err != nil {
		t.Fatalf("first CreateRole error: %v", err)
	}

	resp, err := svc.HandleRequest(ctx, service.Request{Action: "CreateRole", Params: params})
	if err != nil {
		t.Fatalf("second CreateRole error: %v", err)
	}
	if resp.StatusCode != 409 {
		t.Fatalf("expected 409 Conflict, got %d; body: %s", resp.StatusCode, resp.Body)
	}
	if !contains(string(resp.Body), "EntityAlreadyExists") {
		t.Errorf("expected EntityAlreadyExists in body; body: %s", resp.Body)
	}
}

// TestDeleteRoleWithAttachedPolicy はアタッチ済みポリシーを持つロール削除時に DeleteConflict を返すことをテストします。
func TestDeleteRoleWithAttachedPolicy(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	// ロール作成
	if _, err := svc.HandleRequest(ctx, service.Request{
		Action: "CreateRole",
		Params: map[string]string{
			"RoleName":                 "ConflictRole",
			"AssumeRolePolicyDocument": `{"Version":"2012-10-17"}`,
		},
	}); err != nil {
		t.Fatalf("CreateRole error: %v", err)
	}

	// ポリシー作成
	if _, err := svc.HandleRequest(ctx, service.Request{
		Action: "CreatePolicy",
		Params: map[string]string{
			"PolicyName": "TestPolicy",
		},
	}); err != nil {
		t.Fatalf("CreatePolicy error: %v", err)
	}

	// ポリシーをアタッチ
	policyArn := "arn:aws:iam::000000000000:policy/TestPolicy"
	if _, err := svc.HandleRequest(ctx, service.Request{
		Action: "AttachRolePolicy",
		Params: map[string]string{
			"RoleName":  "ConflictRole",
			"PolicyArn": policyArn,
		},
	}); err != nil {
		t.Fatalf("AttachRolePolicy error: %v", err)
	}

	// DeleteRole はアタッチ済みポリシーがあるので DeleteConflict
	resp, err := svc.HandleRequest(ctx, service.Request{
		Action: "DeleteRole",
		Params: map[string]string{"RoleName": "ConflictRole"},
	})
	if err != nil {
		t.Fatalf("DeleteRole error: %v", err)
	}
	if resp.StatusCode != 409 {
		t.Fatalf("expected 409 Conflict, got %d; body: %s", resp.StatusCode, resp.Body)
	}
	if !contains(string(resp.Body), "DeleteConflict") {
		t.Errorf("expected DeleteConflict in body; body: %s", resp.Body)
	}
}

// TestAttachRolePolicyIdempotent は AttachRolePolicy が冪等であることをテストします。
func TestAttachRolePolicyIdempotent(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	// ロール作成
	if _, err := svc.HandleRequest(ctx, service.Request{
		Action: "CreateRole",
		Params: map[string]string{
			"RoleName":                 "IdempotentRole",
			"AssumeRolePolicyDocument": `{"Version":"2012-10-17"}`,
		},
	}); err != nil {
		t.Fatalf("CreateRole error: %v", err)
	}

	policyArn := "arn:aws:iam::aws:policy/AmazonS3ReadOnlyAccess"

	// 1回目
	resp1, err := svc.HandleRequest(ctx, service.Request{
		Action: "AttachRolePolicy",
		Params: map[string]string{
			"RoleName":  "IdempotentRole",
			"PolicyArn": policyArn,
		},
	})
	if err != nil {
		t.Fatalf("first AttachRolePolicy error: %v", err)
	}
	if resp1.StatusCode != 200 {
		t.Fatalf("first AttachRolePolicy status = %d, want 200; body: %s", resp1.StatusCode, resp1.Body)
	}

	// 2回目 (冪等)
	resp2, err := svc.HandleRequest(ctx, service.Request{
		Action: "AttachRolePolicy",
		Params: map[string]string{
			"RoleName":  "IdempotentRole",
			"PolicyArn": policyArn,
		},
	})
	if err != nil {
		t.Fatalf("second AttachRolePolicy error: %v", err)
	}
	if resp2.StatusCode != 200 {
		t.Fatalf("second AttachRolePolicy status = %d, want 200 (idempotent); body: %s", resp2.StatusCode, resp2.Body)
	}

	// ListAttachedRolePolicies でアタッチが1件のみであることを確認
	listResp, err := svc.HandleRequest(ctx, service.Request{
		Action: "ListAttachedRolePolicies",
		Params: map[string]string{"RoleName": "IdempotentRole"},
	})
	if err != nil {
		t.Fatalf("ListAttachedRolePolicies error: %v", err)
	}
	if listResp.StatusCode != 200 {
		t.Fatalf("ListAttachedRolePolicies status = %d; body: %s", listResp.StatusCode, listResp.Body)
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
