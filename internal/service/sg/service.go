package sg

import (
	"context"
	"fmt"
	"net/http"

	"github.com/HMasataka/cloudia/internal/config"
	awsprot "github.com/HMasataka/cloudia/internal/protocol/aws"
	"github.com/HMasataka/cloudia/internal/service"
	"go.uber.org/zap"
)

// SGService は AWS EC2 SecurityGroup サービスのエミュレーションを行います。
// セキュリティグループのメタデータ管理のみを行います（Docker iptables 不使用）。
type SGService struct {
	cfg    config.AWSAuthConfig
	store  service.Store
	logger *zap.Logger
}

// NewSGService は新しい SGService を返します。
func NewSGService(cfg config.AWSAuthConfig, logger *zap.Logger) *SGService {
	return &SGService{
		cfg:    cfg,
		logger: logger,
	}
}

// Name はサービス名を返します。
func (s *SGService) Name() string {
	return "sg"
}

// Provider はプロバイダ名を返します。
func (s *SGService) Provider() string {
	return "aws"
}

// Init はサービスを初期化します。
func (s *SGService) Init(_ context.Context, deps service.ServiceDeps) error {
	s.store = deps.Store
	return nil
}

// HandleRequest はアクションに応じてリクエストを処理します。
func (s *SGService) HandleRequest(ctx context.Context, req service.Request) (service.Response, error) {
	switch req.Action {
	case "CreateSecurityGroup":
		return s.createSecurityGroup(ctx, req)
	case "DeleteSecurityGroup":
		return s.deleteSecurityGroup(ctx, req)
	case "DescribeSecurityGroups":
		return s.describeSecurityGroups(ctx, req)
	case "AuthorizeSecurityGroupIngress":
		return s.authorizeSecurityGroupIngress(ctx, req)
	case "RevokeSecurityGroupIngress":
		return s.revokeSecurityGroupIngress(ctx, req)
	case "AuthorizeSecurityGroupEgress":
		return s.authorizeSecurityGroupEgress(ctx, req)
	case "RevokeSecurityGroupEgress":
		return s.revokeSecurityGroupEgress(ctx, req)
	default:
		return errorResponse(http.StatusBadRequest, "UnsupportedOperation",
			fmt.Sprintf("The action %q is not supported by this service.", req.Action))
	}
}

// SupportedActions はこのサービスがサポートするアクション名の一覧を返します。
func (s *SGService) SupportedActions() []string {
	return []string{
		"CreateSecurityGroup",
		"DeleteSecurityGroup",
		"DescribeSecurityGroups",
		"AuthorizeSecurityGroupIngress",
		"RevokeSecurityGroupIngress",
		"AuthorizeSecurityGroupEgress",
		"RevokeSecurityGroupEgress",
	}
}

// Health はサービスのヘルスステータスを返します。
func (s *SGService) Health(_ context.Context) service.HealthStatus {
	return service.HealthStatus{Healthy: true, Message: "ok"}
}

// Shutdown はサービスを終了します。
func (s *SGService) Shutdown(_ context.Context) error {
	return nil
}

// errorResponse は AWS 互換の XML エラーレスポンスを返します。
func errorResponse(statusCode int, code, message string) (service.Response, error) {
	resp, err := awsprot.MarshalXMLResponse(statusCode, awsprot.ErrorResponse{
		Error: awsprot.ErrorDetail{
			Code:    code,
			Message: message,
		},
		RequestID: "cloudia-sg",
	}, xmlNamespace)
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}
	return resp, nil
}
