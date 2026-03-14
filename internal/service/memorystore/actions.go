package memorystore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
)

// instanceStoreID は project+location+name をキーとする store ID を生成します。
func instanceStoreID(project, location, name string) string {
	return fmt.Sprintf("%s/%s/%s", project, location, name)
}

// instanceFullName は Memorystore の完全リソース名を生成します。
func instanceFullName(project, location, name string) string {
	return fmt.Sprintf("projects/%s/locations/%s/instances/%s", project, location, name)
}

// createInstance は instances.create を処理します。
func (m *MemorystoreService) createInstance(ctx context.Context, req service.Request, project, location string) (service.Response, error) {
	var body CreateInstanceRequest
	if err := json.Unmarshal(req.Body, &body); err != nil {
		return memorystoreErrorResponse(http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
	}

	if body.Name == "" {
		return memorystoreErrorResponse(http.StatusBadRequest, "instance name is required")
	}

	storeID := instanceStoreID(project, location, body.Name)

	// 重複チェック
	existing, err := m.store.Get(ctx, kindInstance, storeID)
	if err == nil && existing != nil {
		return memorystoreErrorResponse(http.StatusConflict,
			fmt.Sprintf("Resource '%s' already exists", instanceFullName(project, location, body.Name)))
	}

	tier := body.Tier
	if tier == "" {
		tier = "BASIC"
	}
	memorySizeGb := body.MemorySizeGb
	if memorySizeGb <= 0 {
		memorySizeGb = 1
	}
	redisVersion := body.RedisVersion
	if redisVersion == "" {
		redisVersion = "REDIS_7_0"
	}

	port := 6379
	if m.redisPort != "" {
		if p, err := strconv.Atoi(m.redisPort); err == nil {
			port = p
		}
	}

	now := time.Now().UTC()
	resource := &models.Resource{
		Kind:      kindInstance,
		ID:        storeID,
		Provider:  "gcp",
		Service:   "memorystore",
		Region:    location,
		Status:    statusReady,
		CreatedAt: now,
		UpdatedAt: now,
		Spec: map[string]interface{}{
			"name":         body.Name,
			"tier":         tier,
			"memorySizeGb": memorySizeGb,
			"redisVersion": redisVersion,
			"project":      project,
			"location":     location,
			"host":         m.redisHost,
			"port":         port,
			"createTime":   now.Format(time.RFC3339),
		},
	}

	if err := m.store.Put(ctx, resource); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	item := instanceItemFromResource(resource, project, location)
	op := Operation{
		Name:     fmt.Sprintf("projects/%s/locations/%s/operations/create-%s", project, location, body.Name),
		Done:     true,
		Response: item,
	}
	return jsonResponse(http.StatusOK, op)
}

// getInstance は instances.get を処理します。
func (m *MemorystoreService) getInstance(ctx context.Context, project, location, name string) (service.Response, error) {
	storeID := instanceStoreID(project, location, name)
	r, err := m.store.Get(ctx, kindInstance, storeID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return memorystoreErrorResponse(http.StatusNotFound,
				fmt.Sprintf("Resource '%s' was not found", instanceFullName(project, location, name)))
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	item := instanceItemFromResource(r, project, location)
	return jsonResponse(http.StatusOK, item)
}

// listInstances は instances.list を処理します。
func (m *MemorystoreService) listInstances(ctx context.Context, project, location string) (service.Response, error) {
	all, err := m.store.List(ctx, kindInstance, state.Filter{})
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	prefix := project + "/" + location + "/"
	var items []InstanceItem
	for _, r := range all {
		if strings.HasPrefix(r.ID, prefix) {
			items = append(items, instanceItemFromResource(r, project, location))
		}
	}

	resp := ListInstancesResponse{
		Instances: items,
	}
	return jsonResponse(http.StatusOK, resp)
}

// deleteInstance は instances.delete を処理します。
func (m *MemorystoreService) deleteInstance(ctx context.Context, project, location, name string) (service.Response, error) {
	storeID := instanceStoreID(project, location, name)
	_, err := m.store.Get(ctx, kindInstance, storeID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return memorystoreErrorResponse(http.StatusNotFound,
				fmt.Sprintf("Resource '%s' was not found", instanceFullName(project, location, name)))
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	if err := m.store.Delete(ctx, kindInstance, storeID); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	op := Operation{
		Name: fmt.Sprintf("projects/%s/locations/%s/operations/delete-%s", project, location, name),
		Done: true,
	}
	return jsonResponse(http.StatusOK, op)
}

// updateInstance は instances.patch を処理します。
func (m *MemorystoreService) updateInstance(ctx context.Context, req service.Request, project, location, name string) (service.Response, error) {
	storeID := instanceStoreID(project, location, name)
	r, err := m.store.Get(ctx, kindInstance, storeID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return memorystoreErrorResponse(http.StatusNotFound,
				fmt.Sprintf("Resource '%s' was not found", instanceFullName(project, location, name)))
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	var body UpdateInstanceRequest
	if len(req.Body) > 0 {
		if err := json.Unmarshal(req.Body, &body); err != nil {
			return memorystoreErrorResponse(http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		}
	}

	if body.DisplayName != "" {
		r.Spec["displayName"] = body.DisplayName
	}
	if body.MemorySizeGb > 0 {
		r.Spec["memorySizeGb"] = body.MemorySizeGb
	}
	if body.RedisVersion != "" {
		r.Spec["redisVersion"] = body.RedisVersion
	}

	r.UpdatedAt = time.Now().UTC()

	if err := m.store.Put(ctx, r); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	item := instanceItemFromResource(r, project, location)
	op := Operation{
		Name:     fmt.Sprintf("projects/%s/locations/%s/operations/update-%s", project, location, name),
		Done:     true,
		Response: item,
	}
	return jsonResponse(http.StatusOK, op)
}

// instanceItemFromResource は models.Resource から InstanceItem を構築します。
func instanceItemFromResource(r *models.Resource, project, location string) InstanceItem {
	name, _ := r.Spec["name"].(string)
	tier, _ := r.Spec["tier"].(string)
	redisVersion, _ := r.Spec["redisVersion"].(string)
	host, _ := r.Spec["host"].(string)
	createTime, _ := r.Spec["createTime"].(string)
	displayName, _ := r.Spec["displayName"].(string)

	memorySizeGb := 1
	if v, ok := r.Spec["memorySizeGb"].(int); ok {
		memorySizeGb = v
	}

	port := 6379
	if v, ok := r.Spec["port"].(int); ok {
		port = v
	}

	return InstanceItem{
		Name:         instanceFullName(project, location, name),
		DisplayName:  displayName,
		Tier:         tier,
		MemorySizeGb: memorySizeGb,
		RedisVersion: redisVersion,
		State:        r.Status,
		Host:         host,
		Port:         port,
		CreateTime:   createTime,
	}
}
