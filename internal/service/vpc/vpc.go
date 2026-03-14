package vpc

import (
	"context"
	"fmt"
	"net/http"

	"github.com/HMasataka/cloudia/internal/config"
	awsprot "github.com/HMasataka/cloudia/internal/protocol/aws"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
	"go.uber.org/zap"
)

// VPCService は AWS VPC サービスのエミュレーションを行います。
// Docker ネットワークを VPC のバックエンドとして使用します。
type VPCService struct {
	cfg     config.AWSAuthConfig
	store   service.Store
	network service.NetworkManager
	logger  *zap.Logger
}

// NewVPCService は新しい VPCService を返します。
func NewVPCService(cfg config.AWSAuthConfig, logger *zap.Logger) *VPCService {
	return &VPCService{
		cfg:    cfg,
		logger: logger,
	}
}

// Name はサービス名を返します。
func (v *VPCService) Name() string {
	return "vpc"
}

// Provider はプロバイダ名を返します。
func (v *VPCService) Provider() string {
	return "aws"
}

// Init はサービスを初期化します。
func (v *VPCService) Init(_ context.Context, deps service.ServiceDeps) error {
	v.store = deps.Store
	v.network = deps.NetworkManager
	return nil
}

// HandleRequest はアクションに応じてリクエストを処理します。
func (v *VPCService) HandleRequest(ctx context.Context, req service.Request) (service.Response, error) {
	switch req.Action {
	case "CreateVpc":
		return v.createVpc(ctx, req)
	case "DeleteVpc":
		return v.deleteVpc(ctx, req)
	case "DescribeVpcs":
		return v.describeVpcs(ctx, req)
	case "CreateSubnet":
		return v.createSubnet(ctx, req)
	case "DeleteSubnet":
		return v.deleteSubnet(ctx, req)
	case "DescribeSubnets":
		return v.describeSubnets(ctx, req)
	default:
		return errorResponse(http.StatusBadRequest, "UnsupportedOperation",
			fmt.Sprintf("The action %q is not supported by this service.", req.Action))
	}
}

// SupportedActions はこのサービスがサポートするアクション名の一覧を返します。
func (v *VPCService) SupportedActions() []string {
	return []string{
		"CreateVpc",
		"DeleteVpc",
		"DescribeVpcs",
		"CreateSubnet",
		"DeleteSubnet",
		"DescribeSubnets",
	}
}

// Health はサービスのヘルスステータスを返します。
func (v *VPCService) Health(_ context.Context) service.HealthStatus {
	return service.HealthStatus{Healthy: true, Message: "ok"}
}

// Shutdown は管理中の Docker ネットワークを全て削除します。
func (v *VPCService) Shutdown(ctx context.Context) error {
	if v.store == nil || v.network == nil {
		return nil
	}

	vpcs, err := v.store.List(ctx, kindVPC, state.Filter{})
	if err != nil {
		return fmt.Errorf("vpc shutdown: list vpcs: %w", err)
	}

	var firstErr error
	for _, r := range vpcs {
		networkID, _ := r.Spec["NetworkID"].(string)
		if networkID == "" {
			continue
		}
		if rmErr := v.network.RemoveNetwork(ctx, networkID); rmErr != nil {
			v.logger.Warn("vpc shutdown: remove network failed",
				zap.String("vpc_id", r.ID),
				zap.String("network_id", networkID),
				zap.Error(rmErr),
			)
			if firstErr == nil {
				firstErr = rmErr
			}
		}
	}

	return firstErr
}

// errorResponse は AWS 互換の XML エラーレスポンスを返します。
func errorResponse(statusCode int, code, message string) (service.Response, error) {
	resp, err := awsprot.MarshalXMLResponse(statusCode, awsprot.ErrorResponse{
		Error: awsprot.ErrorDetail{
			Code:    code,
			Message: message,
		},
		RequestID: "cloudia-vpc",
	}, xmlNamespace)
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}
	return resp, nil
}

// toVpcItem は models.Resource から VpcItem を生成します。
func toVpcItem(r *models.Resource) VpcItem {
	cidr, _ := r.Spec["CidrBlock"].(string)
	return VpcItem{
		VpcId:     r.ID,
		CidrBlock: cidr,
		State:     "available",
	}
}

// toSubnetItem は models.Resource から SubnetItem を生成します。
func toSubnetItem(r *models.Resource) SubnetItem {
	cidr, _ := r.Spec["CidrBlock"].(string)
	vpcID, _ := r.Spec["VpcId"].(string)
	return SubnetItem{
		SubnetId:  r.ID,
		VpcId:     vpcID,
		CidrBlock: cidr,
		State:     "available",
	}
}
