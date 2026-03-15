package rds

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"
	"unicode/utf8"

	awsprot "github.com/HMasataka/cloudia/internal/protocol/aws"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
)

var dbNameRegexp = regexp.MustCompile(`^[a-zA-Z0-9_]*$`)

// validateMasterUserPassword は MasterUserPassword のバリデーションを行います。
// MySQL: 8-41 文字、PostgreSQL: 8-128 文字必須です。
func validateMasterUserPassword(password, engine string) error {
	l := utf8.RuneCountInString(password)
	switch engine {
	case "postgres":
		if l < 8 || l > 128 {
			return fmt.Errorf("MasterUserPassword must be between 8 and 128 characters")
		}
	default: // mysql
		if l < 8 || l > 41 {
			return fmt.Errorf("MasterUserPassword must be between 8 and 41 characters")
		}
	}
	return nil
}

// validateDBName は DBName のバリデーションを行います。
// 英数字とアンダースコアのみ、64 文字以内です。
func validateDBName(name string) error {
	if name == "" {
		return nil
	}
	if utf8.RuneCountInString(name) > 64 {
		return fmt.Errorf("DBName must be 64 characters or fewer")
	}
	if !dbNameRegexp.MatchString(name) {
		return fmt.Errorf("DBName must contain only alphanumeric characters and underscores")
	}
	return nil
}

// createDBInstance は CreateDBInstance アクションを処理します。
func (r *RDSService) createDBInstance(ctx context.Context, req service.Request) (service.Response, error) {
	dbInstanceID := req.Params["DBInstanceIdentifier"]
	if dbInstanceID == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter DBInstanceIdentifier.")
	}

	engine := req.Params["Engine"]
	if engine == "" {
		engine = "mysql"
	}
	if engine != "mysql" && engine != "postgres" {
		return errorResponse(http.StatusBadRequest, "InvalidParameterValue",
			fmt.Sprintf("Invalid DB engine: %s. Supported engines are mysql and postgres.", engine))
	}

	masterUserPassword := req.Params["MasterUserPassword"]
	if masterUserPassword == "" {
		return errorResponse(http.StatusBadRequest, "InvalidParameterValue",
			"The parameter MasterUserPassword is required.")
	}
	if err := validateMasterUserPassword(masterUserPassword, engine); err != nil {
		return errorResponse(http.StatusBadRequest, "InvalidParameterValue", err.Error())
	}

	dbName := req.Params["DBName"]
	if err := validateDBName(dbName); err != nil {
		return errorResponse(http.StatusBadRequest, "InvalidParameterValue", err.Error())
	}

	dbInstanceClass := req.Params["DBInstanceClass"]
	if dbInstanceClass == "" {
		dbInstanceClass = "db.t3.micro"
	}
	engineVersion := req.Params["EngineVersion"]
	if engineVersion == "" {
		switch engine {
		case "postgres":
			engineVersion = "16"
		default:
			engineVersion = "8.0"
		}
	}

	masterUsername := req.Params["MasterUsername"]
	if masterUsername == "" {
		masterUsername = "admin"
	}

	allocatedStorage := 20
	if v := req.Params["AllocatedStorage"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			allocatedStorage = n
		}
	}

	multiAZ := false
	if req.Params["MultiAZ"] == "true" {
		multiAZ = true
	}

	// 既存インスタンスの重複チェック
	if _, err := r.store.Get(ctx, kindDBInstance, dbInstanceID); err == nil {
		return errorResponse(http.StatusBadRequest, "DBInstanceAlreadyExists",
			fmt.Sprintf("DB Instance already exists: %s", dbInstanceID))
	}

	backend, backendErr := r.getOrCreateBackend(ctx, engine)
	if backendErr != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, backendErr
	}
	host := backend.Host()
	portStr := backend.Port()
	if portStr == "" {
		return service.Response{StatusCode: http.StatusInternalServerError},
			fmt.Errorf("rds: backend returned empty port for engine %s", engine)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError},
			fmt.Errorf("rds: backend returned invalid port %q for engine %s: %w", portStr, engine, err)
	}

	now := time.Now().UTC()
	resource := &models.Resource{
		Kind:      kindDBInstance,
		ID:        dbInstanceID,
		Provider:  "aws",
		Service:   "rds",
		Region:    r.cfg.Region,
		Status:    statusAvailable,
		CreatedAt: now,
		UpdatedAt: now,
		Spec: map[string]interface{}{
			"DBInstanceClass":      dbInstanceClass,
			"Engine":               engine,
			"EngineVersion":        engineVersion,
			"DBInstanceStatus":     statusAvailable,
			"MasterUsername":       masterUsername,
			"MasterUserPassword":   masterUserPassword,
			"DBName":               dbName,
			"EndpointAddress":      host,
			"EndpointPort":         port,
			"AllocatedStorage":     allocatedStorage,
			"MultiAZ":              multiAZ,
			"PubliclyAccessible":   false,
		},
	}

	if err := r.store.Put(ctx, resource); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	member := dbInstanceMemberFromSpec(dbInstanceID, resource.Spec)
	resp := CreateDBInstanceResult{
		RequestID:  "cloudia-rds",
		DBInstance: member,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// deleteDBInstance は DeleteDBInstance アクションを処理します。
func (r *RDSService) deleteDBInstance(ctx context.Context, req service.Request) (service.Response, error) {
	dbInstanceID := req.Params["DBInstanceIdentifier"]
	if dbInstanceID == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter DBInstanceIdentifier.")
	}

	resource, err := r.store.Get(ctx, kindDBInstance, dbInstanceID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return errorResponse(http.StatusNotFound, "DBInstanceNotFound",
				fmt.Sprintf("DBInstance %s not found.", dbInstanceID))
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	// ステータスを deleting に更新
	resource.Status = statusDeleting
	resource.Spec["DBInstanceStatus"] = statusDeleting
	resource.UpdatedAt = time.Now().UTC()

	member := dbInstanceMemberFromSpec(dbInstanceID, resource.Spec)

	if err := r.store.Delete(ctx, kindDBInstance, dbInstanceID); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	resp := DeleteDBInstanceResult{
		RequestID:  "cloudia-rds",
		DBInstance: member,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// describeDBInstances は DescribeDBInstances アクションを処理します。
func (r *RDSService) describeDBInstances(ctx context.Context, req service.Request) (service.Response, error) {
	dbInstanceID := req.Params["DBInstanceIdentifier"]

	var resources []*models.Resource

	if dbInstanceID != "" {
		resource, err := r.store.Get(ctx, kindDBInstance, dbInstanceID)
		if err != nil {
			if errors.Is(err, models.ErrNotFound) {
				return errorResponse(http.StatusNotFound, "DBInstanceNotFound",
					fmt.Sprintf("DBInstance %s not found.", dbInstanceID))
			}
			return service.Response{StatusCode: http.StatusInternalServerError}, err
		}
		resources = []*models.Resource{resource}
	} else {
		var err error
		resources, err = r.store.List(ctx, kindDBInstance, state.Filter{})
		if err != nil {
			return service.Response{StatusCode: http.StatusInternalServerError}, err
		}
	}

	members := make([]DBInstanceMember, 0, len(resources))
	for _, res := range resources {
		members = append(members, dbInstanceMemberFromSpec(res.ID, res.Spec))
	}

	resp := DescribeDBInstancesResult{
		RequestID:   "cloudia-rds",
		DBInstances: members,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// modifyDBInstance は ModifyDBInstance アクションを処理します。
func (r *RDSService) modifyDBInstance(ctx context.Context, req service.Request) (service.Response, error) {
	dbInstanceID := req.Params["DBInstanceIdentifier"]
	if dbInstanceID == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter DBInstanceIdentifier.")
	}

	resource, err := r.store.Get(ctx, kindDBInstance, dbInstanceID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return errorResponse(http.StatusNotFound, "DBInstanceNotFound",
				fmt.Sprintf("DBInstance %s not found.", dbInstanceID))
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	// 変更可能なフィールドを更新
	if v := req.Params["DBInstanceClass"]; v != "" {
		resource.Spec["DBInstanceClass"] = v
	}
	if v := req.Params["MasterUserPassword"]; v != "" {
		existingEngine, _ := resource.Spec["Engine"].(string)
		if existingEngine == "" {
			existingEngine = "mysql"
		}
		if err := validateMasterUserPassword(v, existingEngine); err != nil {
			return errorResponse(http.StatusBadRequest, "InvalidParameterValue", err.Error())
		}
		resource.Spec["MasterUserPassword"] = v
	}
	if v := req.Params["AllocatedStorage"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			resource.Spec["AllocatedStorage"] = n
		}
	}
	if v := req.Params["MultiAZ"]; v != "" {
		resource.Spec["MultiAZ"] = v == "true"
	}

	resource.UpdatedAt = time.Now().UTC()

	if err := r.store.Put(ctx, resource); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	member := dbInstanceMemberFromSpec(dbInstanceID, resource.Spec)
	resp := ModifyDBInstanceResult{
		RequestID:  "cloudia-rds",
		DBInstance: member,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// createDBSnapshot は CreateDBSnapshot アクションを処理します。
// スナップショットはメタデータのみ (mysqldump は実行しない)。
func (r *RDSService) createDBSnapshot(ctx context.Context, req service.Request) (service.Response, error) {
	dbSnapshotID := req.Params["DBSnapshotIdentifier"]
	if dbSnapshotID == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter DBSnapshotIdentifier.")
	}

	dbInstanceID := req.Params["DBInstanceIdentifier"]
	if dbInstanceID == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter DBInstanceIdentifier.")
	}

	// DB インスタンスの存在確認
	instanceResource, err := r.store.Get(ctx, kindDBInstance, dbInstanceID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return errorResponse(http.StatusNotFound, "DBInstanceNotFound",
				fmt.Sprintf("DBInstance %s not found.", dbInstanceID))
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	// スナップショットの重複チェック
	if _, err := r.store.Get(ctx, kindDBSnapshot, dbSnapshotID); err == nil {
		return errorResponse(http.StatusBadRequest, "DBSnapshotAlreadyExists",
			fmt.Sprintf("Cannot create the snapshot because a snapshot with the identifier %s already exists.", dbSnapshotID))
	}

	engine, _ := instanceResource.Spec["Engine"].(string)
	engineVersion, _ := instanceResource.Spec["EngineVersion"].(string)
	allocatedStorage, _ := instanceResource.Spec["AllocatedStorage"].(int)
	if allocatedStorage == 0 {
		if v, ok := instanceResource.Spec["AllocatedStorage"].(float64); ok {
			allocatedStorage = int(v)
		}
	}

	now := time.Now().UTC()
	snapshotResource := &models.Resource{
		Kind:      kindDBSnapshot,
		ID:        dbSnapshotID,
		Provider:  "aws",
		Service:   "rds",
		Region:    r.cfg.Region,
		Status:    "available",
		CreatedAt: now,
		UpdatedAt: now,
		Spec: map[string]interface{}{
			"DBInstanceIdentifier": dbInstanceID,
			"Engine":               engine,
			"EngineVersion":        engineVersion,
			"Status":               "available",
			"SnapshotType":         "manual",
			"AllocatedStorage":     allocatedStorage,
		},
	}

	if err := r.store.Put(ctx, snapshotResource); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	member := dbSnapshotMemberFromSpec(dbSnapshotID, snapshotResource.Spec)
	resp := CreateDBSnapshotResult{
		RequestID:  "cloudia-rds",
		DBSnapshot: member,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// describeDBSnapshots は DescribeDBSnapshots アクションを処理します。
func (r *RDSService) describeDBSnapshots(ctx context.Context, req service.Request) (service.Response, error) {
	dbSnapshotID := req.Params["DBSnapshotIdentifier"]
	dbInstanceID := req.Params["DBInstanceIdentifier"]

	var resources []*models.Resource

	if dbSnapshotID != "" {
		resource, err := r.store.Get(ctx, kindDBSnapshot, dbSnapshotID)
		if err != nil {
			if errors.Is(err, models.ErrNotFound) {
				return errorResponse(http.StatusNotFound, "DBSnapshotNotFound",
					fmt.Sprintf("DBSnapshot %s not found.", dbSnapshotID))
			}
			return service.Response{StatusCode: http.StatusInternalServerError}, err
		}
		resources = []*models.Resource{resource}
	} else {
		var err error
		resources, err = r.store.List(ctx, kindDBSnapshot, state.Filter{})
		if err != nil {
			return service.Response{StatusCode: http.StatusInternalServerError}, err
		}

		// DBInstanceIdentifier フィルタ
		if dbInstanceID != "" {
			filtered := resources[:0]
			for _, res := range resources {
				if v, ok := res.Spec["DBInstanceIdentifier"].(string); ok && v == dbInstanceID {
					filtered = append(filtered, res)
				}
			}
			resources = filtered
		}
	}

	members := make([]DBSnapshotMember, 0, len(resources))
	for _, res := range resources {
		members = append(members, dbSnapshotMemberFromSpec(res.ID, res.Spec))
	}

	resp := DescribeDBSnapshotsResult{
		RequestID:   "cloudia-rds",
		DBSnapshots: members,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// deleteDBSnapshot は DeleteDBSnapshot アクションを処理します。
func (r *RDSService) deleteDBSnapshot(ctx context.Context, req service.Request) (service.Response, error) {
	dbSnapshotID := req.Params["DBSnapshotIdentifier"]
	if dbSnapshotID == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter DBSnapshotIdentifier.")
	}

	resource, err := r.store.Get(ctx, kindDBSnapshot, dbSnapshotID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return errorResponse(http.StatusNotFound, "DBSnapshotNotFound",
				fmt.Sprintf("DBSnapshot %s not found.", dbSnapshotID))
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	member := dbSnapshotMemberFromSpec(dbSnapshotID, resource.Spec)

	if err := r.store.Delete(ctx, kindDBSnapshot, dbSnapshotID); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	resp := DeleteDBSnapshotResult{
		RequestID:  "cloudia-rds",
		DBSnapshot: member,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}
