package gce

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/backend/docker"
	"github.com/HMasataka/cloudia/internal/backend/mapping"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
)

// instanceStoreID は project+zone+name をキーとする store ID を生成します。
func instanceStoreID(project, zone, name string) string {
	return fmt.Sprintf("%s/%s/%s", project, zone, name)
}

// insertInstance は instances.insert を処理します。
func (g *GCEService) insertInstance(ctx context.Context, req service.Request, project, zone string) (service.Response, error) {
	var body InsertInstanceRequest
	if err := json.Unmarshal(req.Body, &body); err != nil {
		return gceErrorResponse(http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
	}

	if body.Name == "" {
		return gceErrorResponse(http.StatusBadRequest, "instance name is required")
	}

	storeID := instanceStoreID(project, zone, body.Name)

	// 重複チェック
	existing, err := g.store.Get(ctx, kindInstance, storeID)
	if err == nil && existing != nil {
		return gceErrorResponse(http.StatusConflict,
			fmt.Sprintf("The resource 'projects/%s/zones/%s/instances/%s' already exists", project, zone, body.Name))
	}

	// machineType の解決: フルパス or 短縮名の両方をサポート
	machineTypeName := body.MachineType
	if idx := strings.LastIndex(machineTypeName, "/"); idx >= 0 {
		machineTypeName = machineTypeName[idx+1:]
	}

	machineSpec, err := mapping.ResolveMachineType(machineTypeName)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return gceErrorResponse(http.StatusBadRequest,
				fmt.Sprintf("Invalid value for machine type: %q", body.MachineType))
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	// イメージファミリーを解決
	sourceImage := ""
	if len(body.Disks) > 0 && body.Disks[0].InitializeParams != nil {
		sourceImage = body.Disks[0].InitializeParams.SourceImage
	}
	dockerImage := mapping.ResolveGCEDockerImage(sourceImage)

	// コンテナ起動
	if g.limiter != nil {
		if limErr := g.limiter.CheckContainerLimit(ctx); limErr != nil {
			return gceErrorResponse(http.StatusServiceUnavailable, "no capacity available for the requested machine type")
		}
		if limErr := g.limiter.CheckDiskUsage(ctx); limErr != nil {
			return gceErrorResponse(http.StatusServiceUnavailable, "insufficient disk capacity to start the requested instance")
		}
	}

	containerName := "cloudia-gce-" + project + "-" + zone + "-" + body.Name
	labels := docker.ManagedLabels("compute", "gcp", kindInstance, zone)
	labels[docker.LabelResourceID] = storeID

	cfg := docker.ContainerConfig{
		Image:       dockerImage,
		Name:        containerName,
		Labels:      labels,
		CPULimit:    machineSpec.CPU,
		MemoryLimit: machineSpec.Memory,
	}

	containerID, runErr := g.docker.RunContainer(ctx, cfg)
	if runErr != nil {
		g.logger.Warn("gce insertInstance: RunContainer failed",
			zap.String("name", body.Name),
			zap.Error(runErr),
		)
		return gceErrorResponse(http.StatusInternalServerError, "failed to start instance backend")
	}

	info, inspectErr := g.docker.InspectContainer(ctx, containerID)
	if inspectErr != nil {
		g.logger.Warn("gce insertInstance: InspectContainer failed",
			zap.String("name", body.Name),
			zap.Error(inspectErr),
		)
	}
	privateIP := info.IPAddress

	now := time.Now().UTC()
	resource := &models.Resource{
		Kind:        kindInstance,
		ID:          storeID,
		Provider:    "gcp",
		Service:     "compute",
		Region:      zone,
		Status:      statusRunning,
		ContainerID: containerID,
		CreatedAt:   now,
		UpdatedAt:   now,
		Spec: map[string]interface{}{
			"name":        body.Name,
			"machineType": body.MachineType,
			"sourceImage": sourceImage,
			"project":     project,
			"zone":        zone,
			"privateIP":   privateIP,
			"createdAt":   now.Format(time.RFC3339),
		},
	}

	if putErr := g.store.Put(ctx, resource); putErr != nil {
		g.logger.Warn("gce insertInstance: store.Put failed",
			zap.String("name", body.Name),
			zap.Error(putErr),
		)
		_ = g.docker.StopContainer(ctx, containerID, nil)
		_ = g.docker.RemoveContainer(ctx, containerID)
		return service.Response{StatusCode: http.StatusInternalServerError}, putErr
	}

	op := Operation{
		Kind:          "compute#operation",
		Name:          "operation-insert-" + body.Name,
		OperationType: "insert",
		Status:        "DONE",
		TargetID:      storeID,
		TargetLink:    fmt.Sprintf("projects/%s/zones/%s/instances/%s", project, zone, body.Name),
		Zone:          fmt.Sprintf("projects/%s/zones/%s", project, zone),
	}
	return jsonResponse(http.StatusOK, op)
}

// getInstance は instances.get を処理します。
func (g *GCEService) getInstance(ctx context.Context, project, zone, name string) (service.Response, error) {
	storeID := instanceStoreID(project, zone, name)
	r, err := g.store.Get(ctx, kindInstance, storeID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return gceErrorResponse(http.StatusNotFound,
				fmt.Sprintf("The resource 'projects/%s/zones/%s/instances/%s' was not found", project, zone, name))
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	item := instanceItemFromResource(r, project, zone)
	return jsonResponse(http.StatusOK, item)
}

// listInstances は instances.list を処理します。
func (g *GCEService) listInstances(ctx context.Context, project, zone string) (service.Response, error) {
	all, err := g.store.List(ctx, kindInstance, state.Filter{})
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	prefix := project + "/" + zone + "/"
	var items []InstanceItem
	for _, r := range all {
		if strings.HasPrefix(r.ID, prefix) {
			items = append(items, instanceItemFromResource(r, project, zone))
		}
	}

	resp := ListInstancesResponse{
		Kind:  "compute#instanceList",
		Items: items,
	}
	return jsonResponse(http.StatusOK, resp)
}

// deleteInstance は instances.delete を処理します。
func (g *GCEService) deleteInstance(ctx context.Context, project, zone, name string) (service.Response, error) {
	storeID := instanceStoreID(project, zone, name)
	r, err := g.store.Get(ctx, kindInstance, storeID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return gceErrorResponse(http.StatusNotFound,
				fmt.Sprintf("The resource 'projects/%s/zones/%s/instances/%s' was not found", project, zone, name))
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	containerID := r.ContainerID
	if containerID != "" {
		if stopErr := g.docker.StopContainer(ctx, containerID, nil); stopErr != nil {
			g.logger.Warn("gce deleteInstance: stop container failed",
				zap.String("name", name),
				zap.String("container_id", containerID),
				zap.Error(stopErr),
			)
		}
		if rmErr := g.docker.RemoveContainer(ctx, containerID); rmErr != nil {
			g.logger.Warn("gce deleteInstance: remove container failed",
				zap.String("name", name),
				zap.String("container_id", containerID),
				zap.Error(rmErr),
			)
		}
	}

	if delErr := g.store.Delete(ctx, kindInstance, storeID); delErr != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, delErr
	}

	op := Operation{
		Kind:          "compute#operation",
		Name:          "operation-delete-" + name,
		OperationType: "delete",
		Status:        "DONE",
		TargetLink:    fmt.Sprintf("projects/%s/zones/%s/instances/%s", project, zone, name),
		Zone:          fmt.Sprintf("projects/%s/zones/%s", project, zone),
	}
	return jsonResponse(http.StatusOK, op)
}

// startInstance は instances.start を処理します (docker unpause)。
func (g *GCEService) startInstance(ctx context.Context, project, zone, name string) (service.Response, error) {
	storeID := instanceStoreID(project, zone, name)
	r, err := g.store.Get(ctx, kindInstance, storeID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return gceErrorResponse(http.StatusNotFound,
				fmt.Sprintf("The resource 'projects/%s/zones/%s/instances/%s' was not found", project, zone, name))
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	// 既に RUNNING なら冪等に成功
	if r.Status == statusRunning {
		op := Operation{
			Kind:          "compute#operation",
			Name:          "operation-start-" + name,
			OperationType: "start",
			Status:        "DONE",
			TargetLink:    fmt.Sprintf("projects/%s/zones/%s/instances/%s", project, zone, name),
			Zone:          fmt.Sprintf("projects/%s/zones/%s", project, zone),
		}
		return jsonResponse(http.StatusOK, op)
	}

	containerID := r.ContainerID
	if containerID != "" {
		if unErr := g.docker.UnpauseContainer(ctx, containerID); unErr != nil {
			return gceErrorResponse(http.StatusInternalServerError, "internal error")
		}
	}

	r.Status = statusRunning
	r.UpdatedAt = time.Now().UTC()
	if putErr := g.store.Put(ctx, r); putErr != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, putErr
	}

	op := Operation{
		Kind:          "compute#operation",
		Name:          "operation-start-" + name,
		OperationType: "start",
		Status:        "DONE",
		TargetLink:    fmt.Sprintf("projects/%s/zones/%s/instances/%s", project, zone, name),
		Zone:          fmt.Sprintf("projects/%s/zones/%s", project, zone),
	}
	return jsonResponse(http.StatusOK, op)
}

// stopInstance は instances.stop を処理します (docker pause)。
func (g *GCEService) stopInstance(ctx context.Context, project, zone, name string) (service.Response, error) {
	storeID := instanceStoreID(project, zone, name)
	r, err := g.store.Get(ctx, kindInstance, storeID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return gceErrorResponse(http.StatusNotFound,
				fmt.Sprintf("The resource 'projects/%s/zones/%s/instances/%s' was not found", project, zone, name))
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	// 既に TERMINATED (stopped) なら冪等に成功
	if r.Status == statusStopped {
		op := Operation{
			Kind:          "compute#operation",
			Name:          "operation-stop-" + name,
			OperationType: "stop",
			Status:        "DONE",
			TargetLink:    fmt.Sprintf("projects/%s/zones/%s/instances/%s", project, zone, name),
			Zone:          fmt.Sprintf("projects/%s/zones/%s", project, zone),
		}
		return jsonResponse(http.StatusOK, op)
	}

	containerID := r.ContainerID
	if containerID != "" {
		if pErr := g.docker.PauseContainer(ctx, containerID); pErr != nil {
			return gceErrorResponse(http.StatusInternalServerError, "internal error")
		}
	}

	r.Status = statusStopped
	r.UpdatedAt = time.Now().UTC()
	if putErr := g.store.Put(ctx, r); putErr != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, putErr
	}

	op := Operation{
		Kind:          "compute#operation",
		Name:          "operation-stop-" + name,
		OperationType: "stop",
		Status:        "DONE",
		TargetLink:    fmt.Sprintf("projects/%s/zones/%s/instances/%s", project, zone, name),
		Zone:          fmt.Sprintf("projects/%s/zones/%s", project, zone),
	}
	return jsonResponse(http.StatusOK, op)
}

// instanceItemFromResource は models.Resource から InstanceItem を構築します。
func instanceItemFromResource(r *models.Resource, project, zone string) InstanceItem {
	name, _ := r.Spec["name"].(string)
	machineType, _ := r.Spec["machineType"].(string)
	privateIP, _ := r.Spec["privateIP"].(string)
	createdAt, _ := r.Spec["createdAt"].(string)

	item := InstanceItem{
		ID:                r.ID,
		Name:              name,
		MachineType:       machineType,
		Status:            r.Status,
		Zone:              fmt.Sprintf("projects/%s/zones/%s", project, zone),
		CreationTimestamp: createdAt,
	}

	if privateIP != "" {
		item.NetworkInterfaces = []NetworkInterface{
			{NetworkIP: privateIP},
		}
	}

	return item
}
