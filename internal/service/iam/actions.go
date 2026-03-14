package iam

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	protocolaws "github.com/HMasataka/cloudia/internal/protocol/aws"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/pkg/models"
)

const (
	kindRole           = "aws:iam:role"
	kindUser           = "aws:iam:user"
	kindPolicy         = "aws:iam:policy"
	kindRolePolicyAttr = "aws:iam:role-policy-attachment"

	iamProvider = "aws"
	iamService  = "iam"
)

// randomID は crypto/rand を使ってランダムな ID 文字列を生成します。
func randomID(prefix string) string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	return prefix + strings.ToUpper(hex.EncodeToString(b))
}

// formatTime は時刻を ISO8601 形式に変換します。
func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// --- Role ---

func (s *IAMService) createRole(ctx context.Context, req service.Request) (service.Response, error) {
	roleName := req.Params["RoleName"]
	if roleName == "" {
		return errorResponse(http.StatusBadRequest, "ValidationError", "RoleName is required")
	}

	// 重複チェック
	_, err := s.store.Get(ctx, kindRole, roleName)
	if err == nil {
		return errorResponse(http.StatusConflict, "EntityAlreadyExists",
			"Role with name "+roleName+" already exists")
	}
	if !errors.Is(err, models.ErrNotFound) {
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	now := time.Now()
	roleID := randomID("AROA")
	accountID := s.cfg.AccountID
	if accountID == "" {
		accountID = "000000000000"
	}
	// IAM は global サービスなので region は空文字列
	arn := protocolaws.FormatARN("aws", "iam", "", accountID, "role/"+roleName)

	resource := &models.Resource{
		Kind:      kindRole,
		ID:        roleName,
		Provider:  iamProvider,
		Service:   iamService,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
		Spec: map[string]interface{}{
			"RoleId":                   roleID,
			"RoleName":                 roleName,
			"Arn":                      arn,
			"Path":                     req.Params["Path"],
			"AssumeRolePolicyDocument": req.Params["AssumeRolePolicyDocument"],
		},
	}
	if err := s.store.Put(ctx, resource); err != nil {
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	resp := CreateRoleResponse{
		Result: RoleResult{
			RoleID:                   roleID,
			RoleName:                 roleName,
			Arn:                      arn,
			Path:                     req.Params["Path"],
			AssumeRolePolicyDocument: req.Params["AssumeRolePolicyDocument"],
			CreateDate:               formatTime(now),
		},
	}
	return protocolaws.MarshalXMLResponse(http.StatusOK, resp, iamNamespace)
}

func (s *IAMService) getRole(ctx context.Context, req service.Request) (service.Response, error) {
	roleName := req.Params["RoleName"]
	if roleName == "" {
		return errorResponse(http.StatusBadRequest, "ValidationError", "RoleName is required")
	}

	resource, err := s.store.Get(ctx, kindRole, roleName)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return errorResponse(http.StatusNotFound, "NoSuchEntity", "Role "+roleName+" not found")
		}
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	resp := GetRoleResponse{
		Result: roleResultFromSpec(resource.Spec, resource.CreatedAt),
	}
	return protocolaws.MarshalXMLResponse(http.StatusOK, resp, iamNamespace)
}

func (s *IAMService) deleteRole(ctx context.Context, req service.Request) (service.Response, error) {
	roleName := req.Params["RoleName"]
	if roleName == "" {
		return errorResponse(http.StatusBadRequest, "ValidationError", "RoleName is required")
	}

	if _, err := s.store.Get(ctx, kindRole, roleName); err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return errorResponse(http.StatusNotFound, "NoSuchEntity", "Role "+roleName+" not found")
		}
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	// アタッチ済みポリシーがあれば DeleteConflict
	attachments, err := s.store.List(ctx, kindRolePolicyAttr, nil)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}
	for _, a := range attachments {
		if a.Spec["RoleName"] == roleName {
			return errorResponse(http.StatusConflict, "DeleteConflict",
				"Cannot delete role "+roleName+" because it has attached policies")
		}
	}

	if err := s.store.Delete(ctx, kindRole, roleName); err != nil {
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	return protocolaws.MarshalXMLResponse(http.StatusOK, DeleteRoleResponse{}, iamNamespace)
}

func (s *IAMService) listRoles(ctx context.Context, _ service.Request) (service.Response, error) {
	resources, err := s.store.List(ctx, kindRole, nil)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	roles := make([]RoleResult, 0, len(resources))
	for _, r := range resources {
		roles = append(roles, roleResultFromSpec(r.Spec, r.CreatedAt))
	}

	resp := ListRolesResponse{Roles: roles, IsTruncated: false}
	return protocolaws.MarshalXMLResponse(http.StatusOK, resp, iamNamespace)
}

func roleResultFromSpec(spec map[string]interface{}, createdAt time.Time) RoleResult {
	return RoleResult{
		RoleID:                   specStr(spec, "RoleId"),
		RoleName:                 specStr(spec, "RoleName"),
		Arn:                      specStr(spec, "Arn"),
		Path:                     specStr(spec, "Path"),
		AssumeRolePolicyDocument: specStr(spec, "AssumeRolePolicyDocument"),
		CreateDate:               formatTime(createdAt),
	}
}

// --- User ---

func (s *IAMService) createUser(ctx context.Context, req service.Request) (service.Response, error) {
	userName := req.Params["UserName"]
	if userName == "" {
		return errorResponse(http.StatusBadRequest, "ValidationError", "UserName is required")
	}

	_, err := s.store.Get(ctx, kindUser, userName)
	if err == nil {
		return errorResponse(http.StatusConflict, "EntityAlreadyExists",
			"User with name "+userName+" already exists")
	}
	if !errors.Is(err, models.ErrNotFound) {
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	now := time.Now()
	userID := randomID("AIDA")
	accountID := s.cfg.AccountID
	if accountID == "" {
		accountID = "000000000000"
	}
	// IAM は global サービスなので region は空文字列
	arn := protocolaws.FormatARN("aws", "iam", "", accountID, "user/"+userName)

	resource := &models.Resource{
		Kind:      kindUser,
		ID:        userName,
		Provider:  iamProvider,
		Service:   iamService,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
		Spec: map[string]interface{}{
			"UserId":   userID,
			"UserName": userName,
			"Arn":      arn,
			"Path":     req.Params["Path"],
		},
	}
	if err := s.store.Put(ctx, resource); err != nil {
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	resp := CreateUserResponse{
		Result: UserResult{
			UserID:     userID,
			UserName:   userName,
			Arn:        arn,
			Path:       req.Params["Path"],
			CreateDate: formatTime(now),
		},
	}
	return protocolaws.MarshalXMLResponse(http.StatusOK, resp, iamNamespace)
}

func (s *IAMService) getUser(ctx context.Context, req service.Request) (service.Response, error) {
	userName := req.Params["UserName"]
	if userName == "" {
		return errorResponse(http.StatusBadRequest, "ValidationError", "UserName is required")
	}

	resource, err := s.store.Get(ctx, kindUser, userName)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return errorResponse(http.StatusNotFound, "NoSuchEntity", "User "+userName+" not found")
		}
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	resp := GetUserResponse{
		Result: userResultFromSpec(resource.Spec, resource.CreatedAt),
	}
	return protocolaws.MarshalXMLResponse(http.StatusOK, resp, iamNamespace)
}

func (s *IAMService) deleteUser(ctx context.Context, req service.Request) (service.Response, error) {
	userName := req.Params["UserName"]
	if userName == "" {
		return errorResponse(http.StatusBadRequest, "ValidationError", "UserName is required")
	}

	if _, err := s.store.Get(ctx, kindUser, userName); err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return errorResponse(http.StatusNotFound, "NoSuchEntity", "User "+userName+" not found")
		}
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	if err := s.store.Delete(ctx, kindUser, userName); err != nil {
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	return protocolaws.MarshalXMLResponse(http.StatusOK, DeleteUserResponse{}, iamNamespace)
}

func (s *IAMService) listUsers(ctx context.Context, _ service.Request) (service.Response, error) {
	resources, err := s.store.List(ctx, kindUser, nil)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	users := make([]UserResult, 0, len(resources))
	for _, r := range resources {
		users = append(users, userResultFromSpec(r.Spec, r.CreatedAt))
	}

	resp := ListUsersResponse{Users: users, IsTruncated: false}
	return protocolaws.MarshalXMLResponse(http.StatusOK, resp, iamNamespace)
}

func userResultFromSpec(spec map[string]interface{}, createdAt time.Time) UserResult {
	return UserResult{
		UserID:     specStr(spec, "UserId"),
		UserName:   specStr(spec, "UserName"),
		Arn:        specStr(spec, "Arn"),
		Path:       specStr(spec, "Path"),
		CreateDate: formatTime(createdAt),
	}
}

// --- Policy ---

func (s *IAMService) createPolicy(ctx context.Context, req service.Request) (service.Response, error) {
	policyName := req.Params["PolicyName"]
	if policyName == "" {
		return errorResponse(http.StatusBadRequest, "ValidationError", "PolicyName is required")
	}

	_, err := s.store.Get(ctx, kindPolicy, policyName)
	if err == nil {
		return errorResponse(http.StatusConflict, "EntityAlreadyExists",
			"Policy with name "+policyName+" already exists")
	}
	if !errors.Is(err, models.ErrNotFound) {
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	now := time.Now()
	policyID := randomID("ANPA")
	accountID := s.cfg.AccountID
	if accountID == "" {
		accountID = "000000000000"
	}
	// IAM は global サービスなので region は空文字列
	arn := protocolaws.FormatARN("aws", "iam", "", accountID, "policy/"+policyName)
	defaultVersionID := "v1"

	resource := &models.Resource{
		Kind:      kindPolicy,
		ID:        policyName,
		Provider:  iamProvider,
		Service:   iamService,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
		Spec: map[string]interface{}{
			"PolicyId":         policyID,
			"PolicyName":       policyName,
			"Arn":              arn,
			"Path":             req.Params["Path"],
			"Description":      req.Params["Description"],
			"DefaultVersionId": defaultVersionID,
		},
	}
	if err := s.store.Put(ctx, resource); err != nil {
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	resp := CreatePolicyResponse{
		Result: PolicyResult{
			PolicyID:         policyID,
			PolicyName:       policyName,
			Arn:              arn,
			Path:             req.Params["Path"],
			DefaultVersionID: defaultVersionID,
			CreateDate:       formatTime(now),
		},
	}
	return protocolaws.MarshalXMLResponse(http.StatusOK, resp, iamNamespace)
}

func (s *IAMService) getPolicy(ctx context.Context, req service.Request) (service.Response, error) {
	policyArn := req.Params["PolicyArn"]
	if policyArn == "" {
		return errorResponse(http.StatusBadRequest, "ValidationError", "PolicyArn is required")
	}

	policyName := arnToName(policyArn)
	resource, err := s.store.Get(ctx, kindPolicy, policyName)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return errorResponse(http.StatusNotFound, "NoSuchEntity", "Policy "+policyArn+" not found")
		}
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	resp := GetPolicyResponse{
		Result: policyResultFromSpec(resource.Spec, resource.CreatedAt),
	}
	return protocolaws.MarshalXMLResponse(http.StatusOK, resp, iamNamespace)
}

func (s *IAMService) deletePolicy(ctx context.Context, req service.Request) (service.Response, error) {
	policyArn := req.Params["PolicyArn"]
	if policyArn == "" {
		return errorResponse(http.StatusBadRequest, "ValidationError", "PolicyArn is required")
	}

	policyName := arnToName(policyArn)
	if _, err := s.store.Get(ctx, kindPolicy, policyName); err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return errorResponse(http.StatusNotFound, "NoSuchEntity", "Policy "+policyArn+" not found")
		}
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	if err := s.store.Delete(ctx, kindPolicy, policyName); err != nil {
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	return protocolaws.MarshalXMLResponse(http.StatusOK, DeletePolicyResponse{}, iamNamespace)
}

func (s *IAMService) listPolicies(ctx context.Context, _ service.Request) (service.Response, error) {
	resources, err := s.store.List(ctx, kindPolicy, nil)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	policies := make([]PolicyResult, 0, len(resources))
	for _, r := range resources {
		policies = append(policies, policyResultFromSpec(r.Spec, r.CreatedAt))
	}

	resp := ListPoliciesResponse{Policies: policies, IsTruncated: false}
	return protocolaws.MarshalXMLResponse(http.StatusOK, resp, iamNamespace)
}

func policyResultFromSpec(spec map[string]interface{}, createdAt time.Time) PolicyResult {
	return PolicyResult{
		PolicyID:         specStr(spec, "PolicyId"),
		PolicyName:       specStr(spec, "PolicyName"),
		Arn:              specStr(spec, "Arn"),
		Path:             specStr(spec, "Path"),
		DefaultVersionID: specStr(spec, "DefaultVersionId"),
		CreateDate:       formatTime(createdAt),
	}
}

// --- Role-Policy Attachment ---

func (s *IAMService) attachRolePolicy(ctx context.Context, req service.Request) (service.Response, error) {
	roleName := req.Params["RoleName"]
	policyArn := req.Params["PolicyArn"]
	if roleName == "" || policyArn == "" {
		return errorResponse(http.StatusBadRequest, "ValidationError", "RoleName and PolicyArn are required")
	}

	// ロールの存在確認
	if _, err := s.store.Get(ctx, kindRole, roleName); err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return errorResponse(http.StatusNotFound, "NoSuchEntity", "Role "+roleName+" not found")
		}
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	// 冪等: 既にアタッチ済みであれば成功を返す
	attachID := roleName + ":" + policyArn
	_, err := s.store.Get(ctx, kindRolePolicyAttr, attachID)
	if err == nil {
		return protocolaws.MarshalXMLResponse(http.StatusOK, AttachRolePolicyResponse{}, iamNamespace)
	}
	if !errors.Is(err, models.ErrNotFound) {
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	now := time.Now()
	policyName := arnToName(policyArn)
	resource := &models.Resource{
		Kind:      kindRolePolicyAttr,
		ID:        attachID,
		Provider:  iamProvider,
		Service:   iamService,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
		Spec: map[string]interface{}{
			"RoleName":   roleName,
			"PolicyArn":  policyArn,
			"PolicyName": policyName,
		},
	}
	if err := s.store.Put(ctx, resource); err != nil {
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	return protocolaws.MarshalXMLResponse(http.StatusOK, AttachRolePolicyResponse{}, iamNamespace)
}

func (s *IAMService) detachRolePolicy(ctx context.Context, req service.Request) (service.Response, error) {
	roleName := req.Params["RoleName"]
	policyArn := req.Params["PolicyArn"]
	if roleName == "" || policyArn == "" {
		return errorResponse(http.StatusBadRequest, "ValidationError", "RoleName and PolicyArn are required")
	}

	attachID := roleName + ":" + policyArn
	if _, err := s.store.Get(ctx, kindRolePolicyAttr, attachID); err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return errorResponse(http.StatusNotFound, "NoSuchEntity",
				"Policy "+policyArn+" is not attached to role "+roleName)
		}
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	if err := s.store.Delete(ctx, kindRolePolicyAttr, attachID); err != nil {
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	return protocolaws.MarshalXMLResponse(http.StatusOK, DetachRolePolicyResponse{}, iamNamespace)
}

func (s *IAMService) listAttachedRolePolicies(ctx context.Context, req service.Request) (service.Response, error) {
	roleName := req.Params["RoleName"]
	if roleName == "" {
		return errorResponse(http.StatusBadRequest, "ValidationError", "RoleName is required")
	}

	if _, err := s.store.Get(ctx, kindRole, roleName); err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return errorResponse(http.StatusNotFound, "NoSuchEntity", "Role "+roleName+" not found")
		}
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	attachments, err := s.store.List(ctx, kindRolePolicyAttr, nil)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, "ServiceError", err.Error())
	}

	var policies []AttachedPolicyResult
	for _, a := range attachments {
		if specStr(a.Spec, "RoleName") == roleName {
			policies = append(policies, AttachedPolicyResult{
				PolicyArn:  specStr(a.Spec, "PolicyArn"),
				PolicyName: specStr(a.Spec, "PolicyName"),
			})
		}
	}

	resp := ListAttachedRolePoliciesResponse{
		AttachedPolicies: policies,
		IsTruncated:      false,
	}
	return protocolaws.MarshalXMLResponse(http.StatusOK, resp, iamNamespace)
}

// --- ヘルパー ---

// specStr は Spec マップから文字列値を取得します。
func specStr(spec map[string]interface{}, key string) string {
	if spec == nil {
		return ""
	}
	v, ok := spec[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// arnToName は ARN からリソース名部分を取得します。
// 例: "arn:aws:iam::000000000000:policy/MyPolicy" → "MyPolicy"
func arnToName(arn string) string {
	parts := strings.Split(arn, "/")
	if len(parts) < 2 {
		return arn
	}
	return parts[len(parts)-1]
}
