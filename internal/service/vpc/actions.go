package vpc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	awsprot "github.com/HMasataka/cloudia/internal/protocol/aws"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
	"go.uber.org/zap"
)

// generateHex12 は 12 文字の小文字 hex 文字列を生成します。
func generateHex12() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate hex12: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// createVpc は CreateVpc アクションを処理します。
// CidrBlock パラメータを受け取り、Docker ネットワークを作成して VPC リソースを保存します。
func (v *VPCService) createVpc(ctx context.Context, req service.Request) (service.Response, error) {
	cidr := req.Params["CidrBlock"]
	if cidr == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter CidrBlock.")
	}

	suffix, err := generateHex12()
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}
	vpcID := "vpc-" + suffix

	networkName := "cloudia-vpc-" + vpcID
	networkID, err := v.network.CreateNetwork(ctx, networkName, cidr)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, "InternalError",
			fmt.Sprintf("Failed to create Docker network: %v", err))
	}

	now := time.Now().UTC()
	resource := &models.Resource{
		Kind:      kindVPC,
		ID:        vpcID,
		Provider:  "aws",
		Service:   "vpc",
		Status:    "available",
		CreatedAt: now,
		UpdatedAt: now,
		Spec: map[string]interface{}{
			"CidrBlock": cidr,
			"NetworkID": networkID,
		},
	}
	if err := v.store.Put(ctx, resource); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	resp := CreateVpcResponse{
		RequestId: "cloudia-vpc",
		Vpc:       toVpcItem(resource),
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// deleteVpc は DeleteVpc アクションを処理します。
// サブネットが残っている場合は DependencyViolation エラーを返します。
func (v *VPCService) deleteVpc(ctx context.Context, req service.Request) (service.Response, error) {
	vpcID := req.Params["VpcId"]
	if vpcID == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter VpcId.")
	}

	// VPC の存在確認
	r, err := v.store.Get(ctx, kindVPC, vpcID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return errorResponse(http.StatusBadRequest, "InvalidVpcID.NotFound",
				fmt.Sprintf("The vpc ID '%s' does not exist.", vpcID))
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	// サブネットの存在チェック
	subnets, err := v.store.List(ctx, kindSubnet, state.Filter{"spec:VpcId": vpcID})
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}
	if len(subnets) > 0 {
		return errorResponse(http.StatusBadRequest, "DependencyViolation",
			fmt.Sprintf("The vpc '%s' has dependencies and cannot be deleted.", vpcID))
	}

	// Docker ネットワークの削除
	networkID, _ := r.Spec["NetworkID"].(string)
	if networkID != "" {
		if rmErr := v.network.RemoveNetwork(ctx, networkID); rmErr != nil {
			v.logger.Warn("deleteVpc: remove network failed",
				zap.String("vpc_id", vpcID),
				zap.String("network_id", networkID),
				zap.Error(rmErr),
			)
		}
	}

	if err := v.store.Delete(ctx, kindVPC, vpcID); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	resp := DeleteVpcResponse{
		RequestId: "cloudia-vpc",
		Return:    true,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// describeVpcs は DescribeVpcs アクションを処理します。
// Filter.1.Name=vpc-id フィルタに対応します。
func (v *VPCService) describeVpcs(ctx context.Context, req service.Request) (service.Response, error) {
	filters := awsprot.ParseFilters(req.Params)

	var vpcIDs []string
	for _, f := range filters {
		if f.Name == "vpc-id" {
			vpcIDs = append(vpcIDs, f.Values...)
		}
	}

	var vpcs []*models.Resource
	if len(vpcIDs) > 0 {
		for _, id := range vpcIDs {
			r, err := v.store.Get(ctx, kindVPC, id)
			if err != nil {
				// 見つからない場合はスキップ
				continue
			}
			vpcs = append(vpcs, r)
		}
	} else {
		var err error
		vpcs, err = v.store.List(ctx, kindVPC, state.Filter{})
		if err != nil {
			return service.Response{StatusCode: http.StatusInternalServerError}, err
		}
	}

	items := make([]VpcItem, 0, len(vpcs))
	for _, r := range vpcs {
		items = append(items, toVpcItem(r))
	}

	resp := DescribeVpcsResponse{
		RequestId: "cloudia-vpc",
		VpcSet:    items,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// createSubnet は CreateSubnet アクションを処理します。
// VpcId と CidrBlock パラメータを受け取り、サブネットリソースを保存します。
func (v *VPCService) createSubnet(ctx context.Context, req service.Request) (service.Response, error) {
	vpcID := req.Params["VpcId"]
	if vpcID == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter VpcId.")
	}
	cidr := req.Params["CidrBlock"]
	if cidr == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter CidrBlock.")
	}

	// VPC 存在確認
	if _, err := v.store.Get(ctx, kindVPC, vpcID); err != nil {
		return errorResponse(http.StatusBadRequest, "InvalidVpcID.NotFound",
			fmt.Sprintf("The vpc ID '%s' does not exist.", vpcID))
	}

	suffix, err := generateHex12()
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}
	subnetID := "subnet-" + suffix

	now := time.Now().UTC()
	resource := &models.Resource{
		Kind:      kindSubnet,
		ID:        subnetID,
		Provider:  "aws",
		Service:   "vpc",
		Status:    "available",
		CreatedAt: now,
		UpdatedAt: now,
		Spec: map[string]interface{}{
			"CidrBlock": cidr,
			"VpcId":     vpcID,
		},
	}
	if err := v.store.Put(ctx, resource); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	resp := CreateSubnetResponse{
		RequestId: "cloudia-vpc",
		Subnet:    toSubnetItem(resource),
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// deleteSubnet は DeleteSubnet アクションを処理します。
func (v *VPCService) deleteSubnet(ctx context.Context, req service.Request) (service.Response, error) {
	subnetID := req.Params["SubnetId"]
	if subnetID == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter SubnetId.")
	}

	// サブネットの存在確認
	if _, err := v.store.Get(ctx, kindSubnet, subnetID); err != nil {
		return errorResponse(http.StatusBadRequest, "InvalidSubnetID.NotFound",
			fmt.Sprintf("The subnet ID '%s' does not exist.", subnetID))
	}

	if err := v.store.Delete(ctx, kindSubnet, subnetID); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	resp := DeleteSubnetResponse{
		RequestId: "cloudia-vpc",
		Return:    true,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// describeSubnets は DescribeSubnets アクションを処理します。
// Filter.1.Name=vpc-id / Filter.1.Name=subnet-id フィルタに対応します。
func (v *VPCService) describeSubnets(ctx context.Context, req service.Request) (service.Response, error) {
	filters := awsprot.ParseFilters(req.Params)

	var filterByVpcID []string
	var filterBySubnetID []string
	for _, f := range filters {
		switch f.Name {
		case "vpc-id":
			filterByVpcID = append(filterByVpcID, f.Values...)
		case "subnet-id":
			filterBySubnetID = append(filterBySubnetID, f.Values...)
		}
	}

	var subnets []*models.Resource

	switch {
	case len(filterBySubnetID) > 0:
		for _, id := range filterBySubnetID {
			r, err := v.store.Get(ctx, kindSubnet, id)
			if err != nil {
				continue
			}
			subnets = append(subnets, r)
		}
	case len(filterByVpcID) > 0:
		for _, vpcID := range filterByVpcID {
			results, err := v.store.List(ctx, kindSubnet, state.Filter{"spec:VpcId": vpcID})
			if err != nil {
				return service.Response{StatusCode: http.StatusInternalServerError}, err
			}
			subnets = append(subnets, results...)
		}
	default:
		var err error
		subnets, err = v.store.List(ctx, kindSubnet, state.Filter{})
		if err != nil {
			return service.Response{StatusCode: http.StatusInternalServerError}, err
		}
	}

	items := make([]SubnetItem, 0, len(subnets))
	for _, r := range subnets {
		items = append(items, toSubnetItem(r))
	}

	resp := DescribeSubnetsResponse{
		RequestId: "cloudia-vpc",
		SubnetSet: items,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}
