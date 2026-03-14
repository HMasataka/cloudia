package rds

import (
	"context"
	"fmt"
	"net/http"

	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/backend/rdb"
	"github.com/HMasataka/cloudia/internal/config"
	awsprot "github.com/HMasataka/cloudia/internal/protocol/aws"
	"github.com/HMasataka/cloudia/internal/service"
)

// RDSService は AWS RDS サービスのエミュレーションを行います。
// RDBBackend (MySQLEngine) を使用して MySQL コンテナを管理します。
type RDSService struct {
	rdb    *rdb.RDBBackend
	store  service.Store
	cfg    config.AWSAuthConfig
	logger *zap.Logger
}

// NewRDSService は新しい RDSService を返します。
func NewRDSService(cfg config.AWSAuthConfig, logger *zap.Logger) *RDSService {
	return &RDSService{
		rdb:    rdb.NewRDBBackend(&rdb.MySQLEngine{}, logger),
		cfg:    cfg,
		logger: logger,
	}
}

// Name はサービス名を返します。
func (r *RDSService) Name() string {
	return "rds"
}

// Provider はプロバイダ名を返します。
func (r *RDSService) Provider() string {
	return "aws"
}

// Init はサービスを初期化します。RDBBackend を起動し SharedBackend に登録します。
func (r *RDSService) Init(ctx context.Context, deps service.ServiceDeps) error {
	r.store = deps.Store

	if err := r.rdb.Init(ctx, deps); err != nil {
		return fmt.Errorf("rds: init rdb backend: %w", err)
	}

	if deps.Registry != nil {
		deps.Registry.SharedBackend("mysql-host", r.rdb.Host())
		deps.Registry.SharedBackend("mysql-port", r.rdb.Port())
		deps.Registry.SharedBackend("mysql-password", r.rdb.RootPassword())
	}

	return nil
}

// HandleRequest はアクションに応じてリクエストを処理します。
func (r *RDSService) HandleRequest(ctx context.Context, req service.Request) (service.Response, error) {
	switch req.Action {
	case "CreateDBInstance":
		return r.createDBInstance(ctx, req)
	case "DeleteDBInstance":
		return r.deleteDBInstance(ctx, req)
	case "DescribeDBInstances":
		return r.describeDBInstances(ctx, req)
	case "ModifyDBInstance":
		return r.modifyDBInstance(ctx, req)
	case "CreateDBSnapshot":
		return r.createDBSnapshot(ctx, req)
	case "DescribeDBSnapshots":
		return r.describeDBSnapshots(ctx, req)
	case "DeleteDBSnapshot":
		return r.deleteDBSnapshot(ctx, req)
	default:
		return errorResponse(http.StatusBadRequest, "UnsupportedOperation",
			fmt.Sprintf("The action %q is not supported by this service.", req.Action))
	}
}

// SupportedActions はこのサービスがサポートするアクション名の一覧を返します。
func (r *RDSService) SupportedActions() []string {
	return []string{
		"CreateDBInstance",
		"DeleteDBInstance",
		"DescribeDBInstances",
		"ModifyDBInstance",
		"CreateDBSnapshot",
		"DescribeDBSnapshots",
		"DeleteDBSnapshot",
	}
}

// Health はサービスのヘルスステータスを返します。
func (r *RDSService) Health(ctx context.Context) service.HealthStatus {
	return r.rdb.Health(ctx)
}

// Shutdown は RDBBackend (MySQL コンテナ) を停止します。
func (r *RDSService) Shutdown(ctx context.Context) error {
	return r.rdb.Shutdown(ctx)
}

// errorResponse は AWS 互換の XML エラーレスポンスを返します。
func errorResponse(statusCode int, code, message string) (service.Response, error) {
	resp, err := awsprot.MarshalXMLResponse(statusCode, awsprot.ErrorResponse{
		Error: awsprot.ErrorDetail{
			Code:    code,
			Message: message,
		},
		RequestID: "cloudia-rds",
	}, xmlNamespace)
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}
	return resp, nil
}

// dbInstanceMemberFromSpec は Spec から DBInstanceMember を構築します。
func dbInstanceMemberFromSpec(id string, spec map[string]interface{}) DBInstanceMember {
	class, _ := spec["DBInstanceClass"].(string)
	engine, _ := spec["Engine"].(string)
	engineVersion, _ := spec["EngineVersion"].(string)
	status, _ := spec["DBInstanceStatus"].(string)
	masterUsername, _ := spec["MasterUsername"].(string)
	dbName, _ := spec["DBName"].(string)
	endpointAddr, _ := spec["EndpointAddress"].(string)
	endpointPort, _ := spec["EndpointPort"].(int)
	allocatedStorage, _ := spec["AllocatedStorage"].(int)
	multiAZ, _ := spec["MultiAZ"].(bool)
	publiclyAccessible, _ := spec["PubliclyAccessible"].(bool)

	if endpointPort == 0 {
		if v, ok := spec["EndpointPort"].(float64); ok {
			endpointPort = int(v)
		}
	}
	if allocatedStorage == 0 {
		if v, ok := spec["AllocatedStorage"].(float64); ok {
			allocatedStorage = int(v)
		}
	}

	if endpointPort == 0 {
		endpointPort = 3306
	}

	return DBInstanceMember{
		DBInstanceIdentifier: id,
		DBInstanceClass:      class,
		Engine:               engine,
		EngineVersion:        engineVersion,
		DBInstanceStatus:     status,
		MasterUsername:       masterUsername,
		DBName:               dbName,
		Endpoint: Endpoint{
			Address: endpointAddr,
			Port:    endpointPort,
		},
		AllocatedStorage:   allocatedStorage,
		MultiAZ:            multiAZ,
		PubliclyAccessible: publiclyAccessible,
	}
}

// dbSnapshotMemberFromSpec は Spec から DBSnapshotMember を構築します。
func dbSnapshotMemberFromSpec(id string, spec map[string]interface{}) DBSnapshotMember {
	dbInstanceID, _ := spec["DBInstanceIdentifier"].(string)
	engine, _ := spec["Engine"].(string)
	engineVersion, _ := spec["EngineVersion"].(string)
	status, _ := spec["Status"].(string)
	snapshotType, _ := spec["SnapshotType"].(string)
	allocatedStorage, _ := spec["AllocatedStorage"].(int)
	if allocatedStorage == 0 {
		if v, ok := spec["AllocatedStorage"].(float64); ok {
			allocatedStorage = int(v)
		}
	}

	return DBSnapshotMember{
		DBSnapshotIdentifier: id,
		DBInstanceIdentifier: dbInstanceID,
		Engine:               engine,
		EngineVersion:        engineVersion,
		Status:               status,
		SnapshotType:         snapshotType,
		AllocatedStorage:     allocatedStorage,
	}
}
