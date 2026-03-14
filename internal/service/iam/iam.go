package iam

import (
	"context"
	"fmt"
	"net/http"

	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/protocol/aws"
	"github.com/HMasataka/cloudia/internal/service"
	"go.uber.org/zap"
)

// IAMService は AWS IAM をエミュレートするサービスです。
type IAMService struct {
	cfg    config.AWSAuthConfig
	store  service.Store
	logger *zap.Logger
}

// NewIAMService は新しい IAMService を生成します。
func NewIAMService(cfg config.AWSAuthConfig, logger *zap.Logger) *IAMService {
	return &IAMService{
		cfg:    cfg,
		logger: logger,
	}
}

// Name はサービス名を返します。
func (s *IAMService) Name() string {
	return "iam"
}

// Provider はプロバイダ名を返します。
func (s *IAMService) Provider() string {
	return "aws"
}

// Init はサービスを初期化します。
func (s *IAMService) Init(_ context.Context, deps service.ServiceDeps) error {
	s.store = deps.Store
	return nil
}

// HandleRequest は Action に応じた処理を行います。
func (s *IAMService) HandleRequest(ctx context.Context, req service.Request) (service.Response, error) {
	switch req.Action {
	case "CreateRole":
		return s.createRole(ctx, req)
	case "GetRole":
		return s.getRole(ctx, req)
	case "DeleteRole":
		return s.deleteRole(ctx, req)
	case "ListRoles":
		return s.listRoles(ctx, req)
	case "CreateUser":
		return s.createUser(ctx, req)
	case "GetUser":
		return s.getUser(ctx, req)
	case "DeleteUser":
		return s.deleteUser(ctx, req)
	case "ListUsers":
		return s.listUsers(ctx, req)
	case "CreatePolicy":
		return s.createPolicy(ctx, req)
	case "GetPolicy":
		return s.getPolicy(ctx, req)
	case "DeletePolicy":
		return s.deletePolicy(ctx, req)
	case "ListPolicies":
		return s.listPolicies(ctx, req)
	case "AttachRolePolicy":
		return s.attachRolePolicy(ctx, req)
	case "DetachRolePolicy":
		return s.detachRolePolicy(ctx, req)
	case "ListAttachedRolePolicies":
		return s.listAttachedRolePolicies(ctx, req)
	default:
		return errorResponse(http.StatusBadRequest, "UnsupportedOperation",
			fmt.Sprintf("Action %s is not supported", req.Action))
	}
}

// SupportedActions はサポートするアクションの一覧を返します。
func (s *IAMService) SupportedActions() []string {
	return []string{
		"CreateRole",
		"GetRole",
		"DeleteRole",
		"ListRoles",
		"CreateUser",
		"GetUser",
		"DeleteUser",
		"ListUsers",
		"CreatePolicy",
		"GetPolicy",
		"DeletePolicy",
		"ListPolicies",
		"AttachRolePolicy",
		"DetachRolePolicy",
		"ListAttachedRolePolicies",
	}
}

// Health は常に healthy を返します。
func (s *IAMService) Health(_ context.Context) service.HealthStatus {
	return service.HealthStatus{Healthy: true, Message: "ok"}
}

// Shutdown は no-op です。
func (s *IAMService) Shutdown(_ context.Context) error {
	return nil
}

// errorResponse は AWS 互換の XML エラーレスポンスを service.Response として返します。
func errorResponse(statusCode int, code, message string) (service.Response, error) {
	errResp := aws.ErrorResponse{
		Error: aws.ErrorDetail{
			Code:    code,
			Message: message,
		},
		RequestID: "00000000-0000-0000-0000-000000000000",
	}
	resp, err := aws.MarshalXMLResponse(statusCode, errResp, iamNamespace)
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}
	return resp, nil
}
