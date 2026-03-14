package sg

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	awsprot "github.com/HMasataka/cloudia/internal/protocol/aws"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
)

// generateHex17 は 17 文字の小文字 hex 文字列を生成します。
func generateHex17() (string, error) {
	// 9 バイト = 18 hex 文字、先頭 17 文字を使用
	b := make([]byte, 9)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate hex17: %w", err)
	}
	return hex.EncodeToString(b)[:17], nil
}

// parseIpPermissions は IpPermissions.N.* 形式のパラメータをパースします。
func parseIpPermissions(params map[string]string) []map[string]interface{} {
	var result []map[string]interface{}
	for i := 1; ; i++ {
		proto, ok := params[fmt.Sprintf("IpPermissions.%d.IpProtocol", i)]
		if !ok {
			break
		}
		perm := map[string]interface{}{
			"IpProtocol": proto,
		}
		if fp, err := strconv.Atoi(params[fmt.Sprintf("IpPermissions.%d.FromPort", i)]); err == nil {
			perm["FromPort"] = fp
		}
		if tp, err := strconv.Atoi(params[fmt.Sprintf("IpPermissions.%d.ToPort", i)]); err == nil {
			perm["ToPort"] = tp
		}
		var ranges []string
		for j := 1; ; j++ {
			cidr, ok := params[fmt.Sprintf("IpPermissions.%d.IpRanges.%d.CidrIp", i, j)]
			if !ok {
				break
			}
			ranges = append(ranges, cidr)
		}
		perm["IpRanges"] = ranges
		result = append(result, perm)
	}
	return result
}

// toIpPermissionItems は spec から格納された IpPermissions を IpPermission スライスに変換します。
func toIpPermissionItems(raw interface{}) []IpPermission {
	perms, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	result := make([]IpPermission, 0, len(perms))
	for _, p := range perms {
		pm, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		perm := IpPermission{}
		perm.IpProtocol, _ = pm["IpProtocol"].(string)
		if fp, ok := pm["FromPort"].(int); ok {
			perm.FromPort = fp
		}
		if tp, ok := pm["ToPort"].(int); ok {
			perm.ToPort = tp
		}
		if ranges, ok := pm["IpRanges"].([]interface{}); ok {
			for _, r := range ranges {
				if cidr, ok := r.(string); ok {
					perm.IpRanges = append(perm.IpRanges, IpRange{CidrIp: cidr})
				}
			}
		}
		result = append(result, perm)
	}
	return result
}

// toSecurityGroupItem は models.Resource から SecurityGroupItem を生成します。
func toSecurityGroupItem(r *models.Resource) SecurityGroupItem {
	name, _ := r.Spec["GroupName"].(string)
	desc, _ := r.Spec["Description"].(string)
	vpcID, _ := r.Spec["VpcId"].(string)

	item := SecurityGroupItem{
		GroupId:     r.ID,
		GroupName:   name,
		Description: desc,
		VpcId:       vpcID,
	}
	item.IpPermissions = toIpPermissionItems(r.Spec["IpPermissions"])
	item.IpPermissionsEgress = toIpPermissionItems(r.Spec["IpPermissionsEgress"])
	return item
}

// createSecurityGroup は CreateSecurityGroup アクションを処理します。
func (s *SGService) createSecurityGroup(ctx context.Context, req service.Request) (service.Response, error) {
	groupName := req.Params["GroupName"]
	if groupName == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter GroupName.")
	}
	description := req.Params["Description"]
	if description == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter Description.")
	}
	vpcID := req.Params["VpcId"]

	// 同名重複チェック
	existing, err := s.store.List(ctx, kindSecurityGroup, state.Filter{"spec:GroupName": groupName})
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}
	if len(existing) > 0 {
		return errorResponse(http.StatusBadRequest, "InvalidGroup.Duplicate",
			fmt.Sprintf("The security group '%s' already exists.", groupName))
	}

	suffix, err := generateHex17()
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}
	sgID := "sg-" + suffix

	// デフォルト egress ルール (すべての送信を許可)
	defaultEgress := []interface{}{
		map[string]interface{}{
			"IpProtocol": "-1",
			"FromPort":   0,
			"ToPort":     0,
			"IpRanges":   []interface{}{"0.0.0.0/0"},
		},
	}

	now := time.Now().UTC()
	resource := &models.Resource{
		Kind:      kindSecurityGroup,
		ID:        sgID,
		Provider:  "aws",
		Service:   "sg",
		Region:    s.cfg.Region,
		Status:    "available",
		CreatedAt: now,
		UpdatedAt: now,
		Spec: map[string]interface{}{
			"GroupName":           groupName,
			"Description":         description,
			"VpcId":               vpcID,
			"IpPermissions":       []interface{}{},
			"IpPermissionsEgress": defaultEgress,
		},
	}
	if err := s.store.Put(ctx, resource); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	resp := CreateSecurityGroupResponse{
		RequestId: "cloudia-sg",
		GroupId:   sgID,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// deleteSecurityGroup は DeleteSecurityGroup アクションを処理します。
// EC2 インスタンスに紐付いている場合は DependencyViolation を返します。
func (s *SGService) deleteSecurityGroup(ctx context.Context, req service.Request) (service.Response, error) {
	groupID := req.Params["GroupId"]
	if groupID == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter GroupId.")
	}

	// SG の存在確認
	if _, err := s.store.Get(ctx, kindSecurityGroup, groupID); err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return errorResponse(http.StatusBadRequest, "InvalidGroup.NotFound",
				fmt.Sprintf("The security group '%s' does not exist.", groupID))
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	// EC2 インスタンスへの紐付きチェック
	instances, err := s.store.List(ctx, kindEC2Instance, state.Filter{})
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}
	for _, inst := range instances {
		sgIDs, ok := inst.Spec["SecurityGroupIds"].([]interface{})
		if !ok {
			continue
		}
		for _, sid := range sgIDs {
			if s, ok := sid.(string); ok && s == groupID {
				return errorResponse(http.StatusBadRequest, "DependencyViolation",
					fmt.Sprintf("The security group '%s' has dependencies and cannot be deleted.", groupID))
			}
		}
	}

	if err := s.store.Delete(ctx, kindSecurityGroup, groupID); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	resp := DeleteSecurityGroupResponse{
		RequestId: "cloudia-sg",
		Return:    true,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// describeSecurityGroups は DescribeSecurityGroups アクションを処理します。
func (s *SGService) describeSecurityGroups(ctx context.Context, req service.Request) (service.Response, error) {
	// GroupId.N パラメータを解析
	var groupIDs []string
	for i := 1; ; i++ {
		id, ok := req.Params[fmt.Sprintf("GroupId.%d", i)]
		if !ok || id == "" {
			break
		}
		groupIDs = append(groupIDs, id)
	}

	var sgs []*models.Resource
	if len(groupIDs) > 0 {
		for _, id := range groupIDs {
			r, err := s.store.Get(ctx, kindSecurityGroup, id)
			if err != nil {
				continue
			}
			sgs = append(sgs, r)
		}
	} else {
		var err error
		sgs, err = s.store.List(ctx, kindSecurityGroup, state.Filter{})
		if err != nil {
			return service.Response{StatusCode: http.StatusInternalServerError}, err
		}
	}

	items := make([]SecurityGroupItem, 0, len(sgs))
	for _, r := range sgs {
		items = append(items, toSecurityGroupItem(r))
	}

	resp := DescribeSecurityGroupsResponse{
		RequestId:      "cloudia-sg",
		SecurityGroups: items,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// authorizeSecurityGroupIngress は AuthorizeSecurityGroupIngress アクションを処理します。
func (s *SGService) authorizeSecurityGroupIngress(ctx context.Context, req service.Request) (service.Response, error) {
	return s.modifySecurityGroupRules(ctx, req, "IpPermissions")
}

// revokeSecurityGroupIngress は RevokeSecurityGroupIngress アクションを処理します。
func (s *SGService) revokeSecurityGroupIngress(ctx context.Context, req service.Request) (service.Response, error) {
	return s.removeSecurityGroupRules(ctx, req, "IpPermissions")
}

// authorizeSecurityGroupEgress は AuthorizeSecurityGroupEgress アクションを処理します。
func (s *SGService) authorizeSecurityGroupEgress(ctx context.Context, req service.Request) (service.Response, error) {
	return s.modifySecurityGroupRules(ctx, req, "IpPermissionsEgress")
}

// revokeSecurityGroupEgress は RevokeSecurityGroupEgress アクションを処理します。
func (s *SGService) revokeSecurityGroupEgress(ctx context.Context, req service.Request) (service.Response, error) {
	return s.removeSecurityGroupRules(ctx, req, "IpPermissionsEgress")
}

// modifySecurityGroupRules は指定されたルールフィールドにパーミッションを追加します。
func (s *SGService) modifySecurityGroupRules(ctx context.Context, req service.Request, field string) (service.Response, error) {
	groupID := req.Params["GroupId"]
	if groupID == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter GroupId.")
	}

	r, err := s.store.Get(ctx, kindSecurityGroup, groupID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return errorResponse(http.StatusBadRequest, "InvalidGroup.NotFound",
				fmt.Sprintf("The security group '%s' does not exist.", groupID))
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	newPerms := parseIpPermissions(req.Params)
	existing, _ := r.Spec[field].([]interface{})
	for _, perm := range newPerms {
		existing = append(existing, perm)
	}
	r.Spec[field] = existing
	r.UpdatedAt = time.Now().UTC()

	if err := s.store.Put(ctx, r); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	var xmlResp interface{}
	switch req.Action {
	case "AuthorizeSecurityGroupIngress":
		xmlResp = AuthorizeSecurityGroupIngressResponse{RequestId: "cloudia-sg", Return: true}
	case "AuthorizeSecurityGroupEgress":
		xmlResp = AuthorizeSecurityGroupEgressResponse{RequestId: "cloudia-sg", Return: true}
	default:
		xmlResp = AuthorizeSecurityGroupIngressResponse{RequestId: "cloudia-sg", Return: true}
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, xmlResp, xmlNamespace)
}

// ipPermissionMatches は2つのパーミッションが一致するかを判定します。
// プロトコル、FromPort、ToPort、IpRanges の内容が一致する場合に true を返します。
func ipPermissionMatches(existing map[string]interface{}, candidate map[string]interface{}) bool {
	if existing["IpProtocol"] != candidate["IpProtocol"] {
		return false
	}
	if existing["FromPort"] != candidate["FromPort"] {
		return false
	}
	if existing["ToPort"] != candidate["ToPort"] {
		return false
	}
	existingRanges, _ := existing["IpRanges"].([]string)
	candidateRanges, _ := candidate["IpRanges"].([]string)
	if len(existingRanges) != len(candidateRanges) {
		return false
	}
	rangeSet := make(map[string]struct{}, len(existingRanges))
	for _, r := range existingRanges {
		rangeSet[r] = struct{}{}
	}
	for _, r := range candidateRanges {
		if _, ok := rangeSet[r]; !ok {
			return false
		}
	}
	return true
}

// removeSecurityGroupRules は指定されたルールフィールドからパーミッションを削除します。
func (s *SGService) removeSecurityGroupRules(ctx context.Context, req service.Request, field string) (service.Response, error) {
	groupID := req.Params["GroupId"]
	if groupID == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter GroupId.")
	}

	r, err := s.store.Get(ctx, kindSecurityGroup, groupID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return errorResponse(http.StatusBadRequest, "InvalidGroup.NotFound",
				fmt.Sprintf("The security group '%s' does not exist.", groupID))
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	toRevoke := parseIpPermissions(req.Params)
	existing, _ := r.Spec[field].([]interface{})

	filtered := make([]interface{}, 0, len(existing))
	for _, e := range existing {
		em, ok := e.(map[string]interface{})
		if !ok {
			filtered = append(filtered, e)
			continue
		}
		matched := false
		for _, candidate := range toRevoke {
			if ipPermissionMatches(em, candidate) {
				matched = true
				break
			}
		}
		if !matched {
			filtered = append(filtered, e)
		}
	}

	r.Spec[field] = filtered
	r.UpdatedAt = time.Now().UTC()

	if err := s.store.Put(ctx, r); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	var xmlResp interface{}
	switch req.Action {
	case "RevokeSecurityGroupIngress":
		xmlResp = RevokeSecurityGroupIngressResponse{RequestId: "cloudia-sg", Return: true}
	case "RevokeSecurityGroupEgress":
		xmlResp = RevokeSecurityGroupEgressResponse{RequestId: "cloudia-sg", Return: true}
	default:
		xmlResp = RevokeSecurityGroupIngressResponse{RequestId: "cloudia-sg", Return: true}
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, xmlResp, xmlNamespace)
}
