package ec2

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/backend/docker"
	"github.com/HMasataka/cloudia/internal/backend/mapping"
	awsprot "github.com/HMasataka/cloudia/internal/protocol/aws"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
)

// parseInstanceIDs は InstanceId.N パラメータからインスタンス ID リストを取得します。
// キーパターンは InstanceId.1, InstanceId.2, ... です。
func parseInstanceIDs(params map[string]string) []string {
	var ids []string
	for i := 1; ; i++ {
		key := fmt.Sprintf("InstanceId.%d", i)
		id, ok := params[key]
		if !ok || id == "" {
			break
		}
		ids = append(ids, id)
	}
	return ids
}

// stopInstances は StopInstances アクションを処理します。
func (e *EC2Service) stopInstances(ctx context.Context, req service.Request) (service.Response, error) {
	instanceIDs := parseInstanceIDs(req.Params)
	if len(instanceIDs) == 0 {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter InstanceId.")
	}

	changes := make([]InstanceStateChange, 0, len(instanceIDs))

	for _, id := range instanceIDs {
		r, err := e.store.Get(ctx, kindInstance, id)
		if err != nil {
			if errors.Is(err, models.ErrNotFound) {
				return errorResponse(http.StatusBadRequest, "InvalidInstanceID.NotFound",
					fmt.Sprintf("The instance ID '%s' does not exist.", id))
			}
			return service.Response{StatusCode: http.StatusInternalServerError}, err
		}

		prevStatus := r.Status
		prevCode := instanceStatusToCode(prevStatus)

		// terminated なインスタンスへの StopInstances は IncorrectInstanceState エラー
		if prevStatus == stateNameTerminated {
			return errorResponse(http.StatusBadRequest, "IncorrectInstanceState",
				fmt.Sprintf("The instance '%s' is not in a state from which it can be stopped.", id))
		}

		// 既に stopped なら冪等に成功
		if prevStatus == stateNameStopped {
			changes = append(changes, InstanceStateChange{
				InstanceId:    id,
				CurrentState:  InstanceStateItem{Code: stateCodeStopped, Name: stateNameStopped},
				PreviousState: InstanceStateItem{Code: prevCode, Name: prevStatus},
			})
			continue
		}

		containerID := r.ContainerID
		if containerID != "" {
			if err := e.docker.PauseContainer(ctx, containerID); err != nil {
				return errorResponse(http.StatusInternalServerError, "InternalError",
					"internal error")
			}
		}

		r.Status = stateNameStopped
		r.Spec["StateCode"] = stateCodeStopped
		r.Spec["StateName"] = stateNameStopped
		r.UpdatedAt = time.Now().UTC()

		if err := e.store.Put(ctx, r); err != nil {
			return service.Response{StatusCode: http.StatusInternalServerError}, err
		}

		changes = append(changes, InstanceStateChange{
			InstanceId:    id,
			CurrentState:  InstanceStateItem{Code: stateCodeStopped, Name: stateNameStopped},
			PreviousState: InstanceStateItem{Code: prevCode, Name: prevStatus},
		})
	}

	resp := StopInstancesResponse{
		RequestId:    "cloudia-ec2",
		InstancesSet: changes,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// startInstances は StartInstances アクションを処理します。
func (e *EC2Service) startInstances(ctx context.Context, req service.Request) (service.Response, error) {
	instanceIDs := parseInstanceIDs(req.Params)
	if len(instanceIDs) == 0 {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter InstanceId.")
	}

	changes := make([]InstanceStateChange, 0, len(instanceIDs))

	for _, id := range instanceIDs {
		r, err := e.store.Get(ctx, kindInstance, id)
		if err != nil {
			if errors.Is(err, models.ErrNotFound) {
				return errorResponse(http.StatusBadRequest, "InvalidInstanceID.NotFound",
					fmt.Sprintf("The instance ID '%s' does not exist.", id))
			}
			return service.Response{StatusCode: http.StatusInternalServerError}, err
		}

		prevStatus := r.Status
		prevCode := instanceStatusToCode(prevStatus)

		// 既に running なら冪等に成功
		if prevStatus == stateNameRunning {
			changes = append(changes, InstanceStateChange{
				InstanceId:    id,
				CurrentState:  InstanceStateItem{Code: stateCodeRunning, Name: stateNameRunning},
				PreviousState: InstanceStateItem{Code: prevCode, Name: prevStatus},
			})
			continue
		}

		// stopped でない場合は IncorrectInstanceState エラー
		if prevStatus != stateNameStopped {
			return errorResponse(http.StatusBadRequest, "IncorrectInstanceState",
				fmt.Sprintf("The instance '%s' is not in a state from which it can be started.", id))
		}

		containerID := r.ContainerID
		if containerID != "" {
			if err := e.docker.UnpauseContainer(ctx, containerID); err != nil {
				return errorResponse(http.StatusInternalServerError, "InternalError",
					"internal error")
			}
		}

		r.Status = stateNameRunning
		r.Spec["StateCode"] = stateCodeRunning
		r.Spec["StateName"] = stateNameRunning
		r.UpdatedAt = time.Now().UTC()

		if err := e.store.Put(ctx, r); err != nil {
			return service.Response{StatusCode: http.StatusInternalServerError}, err
		}

		changes = append(changes, InstanceStateChange{
			InstanceId:    id,
			CurrentState:  InstanceStateItem{Code: stateCodeRunning, Name: stateNameRunning},
			PreviousState: InstanceStateItem{Code: prevCode, Name: prevStatus},
		})
	}

	resp := StartInstancesResponse{
		RequestId:    "cloudia-ec2",
		InstancesSet: changes,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// privateDNSName は IP アドレスから EC2 プライベート DNS 名を生成します。
func privateDNSName(ip string) string {
	if ip == "" {
		return ""
	}
	dashed := strings.ReplaceAll(ip, ".", "-")
	return fmt.Sprintf("ip-%s.ec2.internal", dashed)
}

// instanceItemFromResource は models.Resource から InstanceItem を構築します。
func instanceItemFromResource(r *models.Resource) InstanceItem {
	imageID, _ := r.Spec["ImageId"].(string)
	instanceType, _ := r.Spec["InstanceType"].(string)
	privateIP, _ := r.Spec["PrivateIpAddress"].(string)

	stateCode := stateCodeRunning
	stateName := stateNameRunning
	if sc, ok := r.Spec["StateCode"].(int); ok {
		stateCode = sc
	}
	if sn, ok := r.Spec["StateName"].(string); ok {
		stateName = sn
	}

	var tagItems []TagItem
	for k, v := range r.Tags {
		tagItems = append(tagItems, TagItem{Key: k, Value: v})
	}

	item := InstanceItem{
		InstanceId:     r.ID,
		ImageId:        imageID,
		InstanceType:   instanceType,
		State:          InstanceStateItem{Code: stateCode, Name: stateName},
		PrivateIp:      privateIP,
		PrivateDNSName: privateDNSName(privateIP),
		TagSet:         TagSet{Items: tagItems},
	}

	if lt, ok := r.Spec["LaunchTime"].(time.Time); ok {
		item.LaunchTime = lt.UTC().Format(time.RFC3339)
	}

	return item
}

// runInstances は RunInstances アクションを処理します。
func (e *EC2Service) runInstances(ctx context.Context, req service.Request) (service.Response, error) {
	// 必須パラメータ検証
	amiID := req.Params["ImageId"]
	if amiID == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter ImageId.")
	}

	// InstanceType のデフォルト
	instanceType := req.Params["InstanceType"]
	if instanceType == "" {
		instanceType = "t2.micro"
	}

	// MinCount / MaxCount のデフォルトと検証
	minCount := 1
	maxCount := 1
	if v := req.Params["MinCount"]; v != "" {
		var n int
		if _, err := fmt.Sscan(v, &n); err == nil {
			minCount = n
		}
	}
	if v := req.Params["MaxCount"]; v != "" {
		var n int
		if _, err := fmt.Sscan(v, &n); err == nil {
			maxCount = n
		}
	}
	if maxCount > 10 {
		return errorResponse(http.StatusBadRequest, "InvalidParameterValue",
			"The value for MaxCount exceeds the limit of 10.")
	}
	if minCount < 1 {
		minCount = 1
	}
	if maxCount < minCount {
		maxCount = minCount
	}

	// コンテナ数制限チェック
	if e.limiter != nil {
		if err := e.limiter.CheckContainerLimit(ctx); err != nil {
			return errorResponse(http.StatusServiceUnavailable, "InsufficientInstanceCapacity",
				"There is no capacity available for the requested instance type.")
		}
	}

	// Docker イメージを解決
	dockerImage := mapping.ResolveDockerImage(amiID)

	// CPU / メモリ制限を取得
	machineSpec, err := mapping.ResolveMachineType(instanceType)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return errorResponse(http.StatusBadRequest, "InvalidParameterValue",
				fmt.Sprintf("Invalid value '%s' for InstanceType.", instanceType))
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	// ReservationId を生成
	reservationHex, err := generateHex17()
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}
	reservationID := "r-" + reservationHex

	region := e.cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	// maxCount 分のインスタンスを起動
	type launchedEntry struct {
		instanceID  string
		containerID string
	}
	var launched []InstanceItem
	var launchedEntries []launchedEntry

	for i := 0; i < maxCount; i++ {
		hex17, hexErr := generateHex17()
		if hexErr != nil {
			e.logger.Warn("runInstances: generateHex17 failed", zap.Error(hexErr))
			break
		}
		instanceID := "i-" + hex17
		containerName := "cloudia-" + instanceID

		labels := docker.ManagedLabels("ec2", "aws", "aws:ec2:instance", region)
		labels[docker.LabelResourceID] = instanceID

		cfg := docker.ContainerConfig{
			Image:       dockerImage,
			Name:        containerName,
			Labels:      labels,
			CPULimit:    machineSpec.CPU,
			MemoryLimit: machineSpec.Memory,
		}

		containerID, runErr := e.docker.RunContainer(ctx, cfg)
		if runErr != nil {
			e.logger.Warn("runInstances: RunContainer failed",
				zap.String("instance_id", instanceID),
				zap.Error(runErr),
			)
			break
		}

		// IP アドレスを取得
		info, inspectErr := e.docker.InspectContainer(ctx, containerID)
		if inspectErr != nil {
			e.logger.Warn("runInstances: InspectContainer failed",
				zap.String("instance_id", instanceID),
				zap.Error(inspectErr),
			)
		}
		privateIP := info.IPAddress

		now := time.Now().UTC()
		resource := &models.Resource{
			Kind:        kindInstance,
			ID:          instanceID,
			Provider:    "aws",
			Service:     "ec2",
			Region:      region,
			Status:      stateNameRunning,
			ContainerID: containerID,
			CreatedAt:   now,
			UpdatedAt:   now,
			Spec: map[string]interface{}{
				"ImageId":          amiID,
				"InstanceType":     instanceType,
				"ReservationId":    reservationID,
				"PrivateIpAddress": privateIP,
				"LaunchTime":       now,
				"StateCode":        stateCodeRunning,
				"StateName":        stateNameRunning,
			},
		}

		if putErr := e.store.Put(ctx, resource); putErr != nil {
			e.logger.Warn("runInstances: store.Put failed",
				zap.String("instance_id", instanceID),
				zap.Error(putErr),
			)
			// コンテナを削除してループを抜ける
			if stopErr := e.docker.StopContainer(ctx, containerID, nil); stopErr != nil {
				e.logger.Warn("runInstances: cleanup stop failed after Put error",
					zap.String("container_id", containerID),
					zap.Error(stopErr),
				)
			}
			if rmErr := e.docker.RemoveContainer(ctx, containerID); rmErr != nil {
				e.logger.Warn("runInstances: cleanup remove failed after Put error",
					zap.String("container_id", containerID),
					zap.Error(rmErr),
				)
			}
			break
		}

		launchedEntries = append(launchedEntries, launchedEntry{instanceID: instanceID, containerID: containerID})
		launched = append(launched, instanceItemFromResource(resource))
	}

	// minCount 分起動できなかった場合はクリーンアップして InsufficientInstanceCapacity を返す
	if len(launched) < minCount {
		for _, entry := range launchedEntries {
			if stopErr := e.docker.StopContainer(ctx, entry.containerID, nil); stopErr != nil {
				e.logger.Warn("runInstances: cleanup stop failed",
					zap.String("container_id", entry.containerID),
					zap.Error(stopErr),
				)
			}
			if rmErr := e.docker.RemoveContainer(ctx, entry.containerID); rmErr != nil {
				e.logger.Warn("runInstances: cleanup remove failed",
					zap.String("container_id", entry.containerID),
					zap.Error(rmErr),
				)
			}
			if delErr := e.store.Delete(ctx, kindInstance, entry.instanceID); delErr != nil {
				e.logger.Warn("runInstances: cleanup store.Delete failed",
					zap.String("instance_id", entry.instanceID),
					zap.Error(delErr),
				)
			}
		}
		return errorResponse(http.StatusInternalServerError, "InsufficientInstanceCapacity",
			"There is no capacity available for the requested instance type.")
	}

	resp := RunInstancesResponse{
		RequestId:     "cloudia-ec2",
		ReservationId: reservationID,
		InstancesSet:  launched,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// describeInstances は DescribeInstances アクションを処理します。
func (e *EC2Service) describeInstances(ctx context.Context, req service.Request) (service.Response, error) {
	filters := awsprot.ParseFilters(req.Params)
	instanceIDs := parseInstanceIDs(req.Params)

	// instance-id フィルタからも ID を収集
	var stateFilter []string
	for _, f := range filters {
		switch f.Name {
		case "instance-id":
			instanceIDs = append(instanceIDs, f.Values...)
		case "instance-state-name":
			stateFilter = append(stateFilter, f.Values...)
		}
	}

	var resources []*models.Resource

	if len(instanceIDs) > 0 {
		for _, id := range instanceIDs {
			r, err := e.store.Get(ctx, kindInstance, id)
			if err != nil {
				if errors.Is(err, models.ErrNotFound) {
					return errorResponse(http.StatusBadRequest, "InvalidInstanceID.NotFound",
						fmt.Sprintf("The instance ID '%s' does not exist.", id))
				}
				return service.Response{StatusCode: http.StatusInternalServerError}, err
			}
			resources = append(resources, r)
		}
	} else {
		var err error
		resources, err = e.store.List(ctx, kindInstance, state.Filter{})
		if err != nil {
			return service.Response{StatusCode: http.StatusInternalServerError}, err
		}
	}

	// instance-state-name フィルタで絞り込み
	if len(stateFilter) > 0 {
		stateSet := make(map[string]struct{}, len(stateFilter))
		for _, s := range stateFilter {
			stateSet[s] = struct{}{}
		}
		filtered := resources[:0]
		for _, r := range resources {
			stateName, _ := r.Spec["StateName"].(string)
			if stateName == "" {
				stateName = r.Status
			}
			if _, ok := stateSet[stateName]; ok {
				filtered = append(filtered, r)
			}
		}
		resources = filtered
	}

	// ReservationId でグルーピング
	reservationMap := make(map[string][]InstanceItem)
	var reservationOrder []string

	for _, r := range resources {
		reservationID, _ := r.Spec["ReservationId"].(string)
		if reservationID == "" {
			reservationID = "r-unknown"
		}
		if _, exists := reservationMap[reservationID]; !exists {
			reservationOrder = append(reservationOrder, reservationID)
		}
		reservationMap[reservationID] = append(reservationMap[reservationID], instanceItemFromResource(r))
	}

	reservations := make([]ReservationItem, 0, len(reservationOrder))
	for _, rid := range reservationOrder {
		reservations = append(reservations, ReservationItem{
			ReservationId: rid,
			InstancesSet:  reservationMap[rid],
		})
	}

	resp := DescribeInstancesResponse{
		RequestId:      "cloudia-ec2",
		ReservationSet: reservations,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// terminateInstances は TerminateInstances アクションを処理します。
func (e *EC2Service) terminateInstances(ctx context.Context, req service.Request) (service.Response, error) {
	instanceIDs := parseInstanceIDs(req.Params)
	if len(instanceIDs) == 0 {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter InstanceId.")
	}

	var changes []InstanceStateChange

	for _, id := range instanceIDs {
		r, err := e.store.Get(ctx, kindInstance, id)
		if err != nil {
			if errors.Is(err, models.ErrNotFound) {
				return errorResponse(http.StatusBadRequest, "InvalidInstanceID.NotFound",
					fmt.Sprintf("The instance ID '%s' does not exist.", id))
			}
			return service.Response{StatusCode: http.StatusInternalServerError}, err
		}

		prevStateName := r.Status
		if sn, ok := r.Spec["StateName"].(string); ok && sn != "" {
			prevStateName = sn
		}
		prevStateCode := instanceStatusToCode(prevStateName)

		// 既に terminated なら冪等に成功
		if prevStateName == stateNameTerminated {
			changes = append(changes, InstanceStateChange{
				InstanceId:    id,
				CurrentState:  InstanceStateItem{Code: stateCodeTerminated, Name: stateNameTerminated},
				PreviousState: InstanceStateItem{Code: stateCodeTerminated, Name: stateNameTerminated},
			})
			continue
		}

		// コンテナを停止・削除
		containerID := r.ContainerID
		if containerID != "" {
			if stopErr := e.docker.StopContainer(ctx, containerID, nil); stopErr != nil {
				e.logger.Warn("terminateInstances: stop container failed",
					zap.String("instance_id", id),
					zap.String("container_id", containerID),
					zap.Error(stopErr),
				)
			}
			if rmErr := e.docker.RemoveContainer(ctx, containerID); rmErr != nil {
				e.logger.Warn("terminateInstances: remove container failed",
					zap.String("instance_id", id),
					zap.String("container_id", containerID),
					zap.Error(rmErr),
				)
			}
		}

		// store 上のステータスを terminated に更新 (Delete ではなく Put で保持)
		r.Status = stateNameTerminated
		r.Spec["StateCode"] = stateCodeTerminated
		r.Spec["StateName"] = stateNameTerminated
		r.UpdatedAt = time.Now().UTC()

		if putErr := e.store.Put(ctx, r); putErr != nil {
			return service.Response{StatusCode: http.StatusInternalServerError}, putErr
		}

		changes = append(changes, InstanceStateChange{
			InstanceId:    id,
			CurrentState:  InstanceStateItem{Code: stateCodeTerminated, Name: stateNameTerminated},
			PreviousState: InstanceStateItem{Code: prevStateCode, Name: prevStateName},
		})
	}

	resp := TerminateInstancesResponse{
		RequestId:    "cloudia-ec2",
		InstancesSet: changes,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// instanceStatusToCode はステータス名からステートコードを返します。
func instanceStatusToCode(status string) int {
	switch status {
	case stateNamePending:
		return stateCodePending
	case stateNameRunning:
		return stateCodeRunning
	case stateNameShuttingDown:
		return stateCodeShuttingDown
	case stateNameTerminated:
		return stateCodeTerminated
	case stateNameStopping:
		return stateCodeStopping
	case stateNameStopped:
		return stateCodeStopped
	default:
		return 0
	}
}
