package cloudsql

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

// instanceStoreID は project+name をキーとする store ID を生成します。
func instanceStoreID(project, name string) string {
	return fmt.Sprintf("%s/%s", project, name)
}

// instanceFullName は Cloud SQL の完全リソース名を生成します。
func instanceFullName(project, name string) string {
	return fmt.Sprintf("projects/%s/instances/%s", project, name)
}

// insertInstance は instances.insert を処理します。
func (c *CloudSQLService) insertInstance(ctx context.Context, req service.Request, project string) (service.Response, error) {
	var body InsertInstanceRequest
	if err := json.Unmarshal(req.Body, &body); err != nil {
		return cloudsqlErrorResponse(http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
	}

	if body.Name == "" {
		return cloudsqlErrorResponse(http.StatusBadRequest, "instance name is required")
	}

	storeID := instanceStoreID(project, body.Name)

	// 重複チェック
	existing, err := c.store.Get(ctx, kindInstance, storeID)
	if err == nil && existing != nil {
		return cloudsqlErrorResponse(http.StatusConflict,
			fmt.Sprintf("The Cloud SQL instance '%s' already exists.", body.Name))
	}

	databaseVersion := body.DatabaseVersion
	if databaseVersion == "" {
		databaseVersion = "MYSQL_8_0"
	}

	region := body.Region
	if region == "" {
		region = "us-central1"
	}

	tier := ""
	dataDiskSizeGb := ""
	dataDiskType := ""
	activationPolicy := "ALWAYS"
	if body.Settings != nil {
		if body.Settings.Tier != "" {
			tier = body.Settings.Tier
		}
		if body.Settings.DataDiskSizeGb != "" {
			dataDiskSizeGb = body.Settings.DataDiskSizeGb
		}
		if body.Settings.DataDiskType != "" {
			dataDiskType = body.Settings.DataDiskType
		}
		if body.Settings.ActivationPolicy != "" {
			activationPolicy = body.Settings.ActivationPolicy
		}
	}
	if tier == "" {
		tier = "db-f1-micro"
	}
	if dataDiskSizeGb == "" {
		dataDiskSizeGb = "10"
	}
	if dataDiskType == "" {
		dataDiskType = "PD_SSD"
	}

	// databaseVersion のプレフィックスからエンジン種別を判定する。
	var dbEngine string
	var defaultPort int
	switch {
	case strings.HasPrefix(databaseVersion, "MYSQL_"):
		dbEngine = "mysql"
		defaultPort = 3306
	case strings.HasPrefix(databaseVersion, "POSTGRES_"):
		dbEngine = "postgres"
		defaultPort = 5432
	default:
		return cloudsqlErrorResponse(http.StatusBadRequest,
			fmt.Sprintf("unsupported databaseVersion prefix: %q; only MYSQL_ and POSTGRES_ are supported", databaseVersion))
	}

	// 対応するバックエンドの host/port を取得する。
	backendHost := c.dbHosts[dbEngine]
	backendPort := c.dbPorts[dbEngine]
	if backendHost == "" || backendPort == "" {
		return cloudsqlErrorResponse(http.StatusBadRequest,
			fmt.Sprintf("backend for %q is not available; please create a %s DB instance in RDS first to start the backend", databaseVersion, dbEngine))
	}

	port := defaultPort
	if p, err := strconv.Atoi(backendPort); err == nil {
		port = p
	}

	now := time.Now().UTC()
	resource := &models.Resource{
		Kind:      kindInstance,
		ID:        storeID,
		Provider:  "gcp",
		Service:   "cloudsql",
		Region:    region,
		Status:    statusRunnable,
		CreatedAt: now,
		UpdatedAt: now,
		Spec: map[string]interface{}{
			"name":             body.Name,
			"project":          project,
			"databaseVersion":  databaseVersion,
			"region":           region,
			"state":            statusRunnable,
			"tier":             tier,
			"dataDiskSizeGb":   dataDiskSizeGb,
			"dataDiskType":     dataDiskType,
			"activationPolicy": activationPolicy,
			"ipAddress":        backendHost,
			"port":             port,
			"createTime":       now.Format(time.RFC3339),
		},
	}

	if err := c.store.Put(ctx, resource); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	op := Operation{
		Kind:          "sql#operation",
		Name:          fmt.Sprintf("projects/%s/operations/insert-%s", project, body.Name),
		OperationType: "CREATE",
		Status:        "DONE",
		TargetID:      body.Name,
		TargetLink:    instanceFullName(project, body.Name),
	}
	return jsonResponse(http.StatusOK, op)
}

// getInstance は instances.get を処理します。
func (c *CloudSQLService) getInstance(ctx context.Context, project, name string) (service.Response, error) {
	storeID := instanceStoreID(project, name)
	r, err := c.store.Get(ctx, kindInstance, storeID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return cloudsqlErrorResponse(http.StatusNotFound,
				fmt.Sprintf("The Cloud SQL instance '%s' does not exist.", name))
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	item := instanceItemFromResource(r, project)
	return jsonResponse(http.StatusOK, item)
}

// listInstances は instances.list を処理します。
func (c *CloudSQLService) listInstances(ctx context.Context, project string) (service.Response, error) {
	all, err := c.store.List(ctx, kindInstance, state.Filter{})
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	prefix := project + "/"
	var items []InstanceItem
	for _, r := range all {
		if strings.HasPrefix(r.ID, prefix) {
			items = append(items, instanceItemFromResource(r, project))
		}
	}

	resp := ListInstancesResponse{
		Kind:  "sql#instancesList",
		Items: items,
	}
	return jsonResponse(http.StatusOK, resp)
}

// deleteInstance は instances.delete を処理します。
func (c *CloudSQLService) deleteInstance(ctx context.Context, project, name string) (service.Response, error) {
	storeID := instanceStoreID(project, name)
	_, err := c.store.Get(ctx, kindInstance, storeID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return cloudsqlErrorResponse(http.StatusNotFound,
				fmt.Sprintf("The Cloud SQL instance '%s' does not exist.", name))
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	if err := c.store.Delete(ctx, kindInstance, storeID); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	op := Operation{
		Kind:          "sql#operation",
		Name:          fmt.Sprintf("projects/%s/operations/delete-%s", project, name),
		OperationType: "DELETE",
		Status:        "DONE",
		TargetID:      name,
		TargetLink:    instanceFullName(project, name),
	}
	return jsonResponse(http.StatusOK, op)
}

// updateInstance は instances.patch を処理します。
func (c *CloudSQLService) updateInstance(ctx context.Context, req service.Request, project, name string) (service.Response, error) {
	storeID := instanceStoreID(project, name)
	r, err := c.store.Get(ctx, kindInstance, storeID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return cloudsqlErrorResponse(http.StatusNotFound,
				fmt.Sprintf("The Cloud SQL instance '%s' does not exist.", name))
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	var body UpdateInstanceRequest
	if len(req.Body) > 0 {
		if err := json.Unmarshal(req.Body, &body); err != nil {
			return cloudsqlErrorResponse(http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		}
	}

	if body.DatabaseVersion != "" {
		r.Spec["databaseVersion"] = body.DatabaseVersion
	}
	if body.Settings != nil {
		if body.Settings.Tier != "" {
			r.Spec["tier"] = body.Settings.Tier
		}
		if body.Settings.DataDiskSizeGb != "" {
			r.Spec["dataDiskSizeGb"] = body.Settings.DataDiskSizeGb
		}
		if body.Settings.DataDiskType != "" {
			r.Spec["dataDiskType"] = body.Settings.DataDiskType
		}
		if body.Settings.ActivationPolicy != "" {
			r.Spec["activationPolicy"] = body.Settings.ActivationPolicy
		}
	}

	r.UpdatedAt = time.Now().UTC()

	if err := c.store.Put(ctx, r); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	op := Operation{
		Kind:          "sql#operation",
		Name:          fmt.Sprintf("projects/%s/operations/update-%s", project, name),
		OperationType: "UPDATE",
		Status:        "DONE",
		TargetID:      name,
		TargetLink:    instanceFullName(project, name),
	}
	return jsonResponse(http.StatusOK, op)
}

// instanceItemFromResource は models.Resource から InstanceItem を構築します。
func instanceItemFromResource(r *models.Resource, project string) InstanceItem {
	name, _ := r.Spec["name"].(string)
	databaseVersion, _ := r.Spec["databaseVersion"].(string)
	region, _ := r.Spec["region"].(string)
	ipAddress, _ := r.Spec["ipAddress"].(string)
	createTime, _ := r.Spec["createTime"].(string)
	tier, _ := r.Spec["tier"].(string)
	dataDiskSizeGb, _ := r.Spec["dataDiskSizeGb"].(string)
	dataDiskType, _ := r.Spec["dataDiskType"].(string)
	activationPolicy, _ := r.Spec["activationPolicy"].(string)

	item := InstanceItem{
		Kind:            "sql#instance",
		Name:            name,
		Project:         project,
		DatabaseVersion: databaseVersion,
		Region:          region,
		State:           r.Status,
		CreateTime:      createTime,
		Settings: InstanceSettings{
			Tier:             tier,
			DataDiskSizeGb:   dataDiskSizeGb,
			DataDiskType:     dataDiskType,
			ActivationPolicy: activationPolicy,
		},
	}

	if ipAddress != "" {
		item.IPAddresses = []IPMapping{
			{Type: "PRIMARY", IPAddress: ipAddress},
		}
	}

	return item
}
