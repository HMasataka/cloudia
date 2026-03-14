package rds

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/backend/rdb"
	"github.com/HMasataka/cloudia/internal/config"
	awsprot "github.com/HMasataka/cloudia/internal/protocol/aws"
	"github.com/HMasataka/cloudia/internal/service"
)

// RDSService は AWS RDS サービスのエミュレーションを行います。
// 複数の RDBBackend を管理し、エンジン別に遅延起動します。
type RDSService struct {
	mu       sync.Mutex
	backends map[string]*rdb.RDBBackend
	deps     service.ServiceDeps
	store    service.Store
	cfg      config.AWSAuthConfig
	logger   *zap.Logger
}

// NewRDSService は新しい RDSService を返します。
func NewRDSService(cfg config.AWSAuthConfig, logger *zap.Logger) *RDSService {
	return &RDSService{
		backends: make(map[string]*rdb.RDBBackend),
		cfg:      cfg,
		logger:   logger,
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

// Init はサービスを初期化します。MySQL バックエンドを起動し SharedBackend に登録します。
func (r *RDSService) Init(ctx context.Context, deps service.ServiceDeps) error {
	r.store = deps.Store
	r.deps = deps

	mysqlBackend := rdb.NewRDBBackend(&rdb.MySQLEngine{}, r.logger)
	if err := mysqlBackend.Init(ctx, deps); err != nil {
		return fmt.Errorf("rds: init mysql backend: %w", err)
	}

	r.mu.Lock()
	r.backends["mysql"] = mysqlBackend
	r.mu.Unlock()

	if deps.Registry != nil {
		deps.Registry.SharedBackend("rdb-mysql-host", mysqlBackend.Host())
		deps.Registry.SharedBackend("rdb-mysql-port", mysqlBackend.Port())
		deps.Registry.SharedBackend("rdb-mysql-password", mysqlBackend.RootPassword())
		// 後方互換のため旧キーでも二重登録する
		deps.Registry.SharedBackend("mysql-host", mysqlBackend.Host())
		deps.Registry.SharedBackend("mysql-port", mysqlBackend.Port())
		deps.Registry.SharedBackend("mysql-password", mysqlBackend.RootPassword())
	}

	return nil
}

// getOrCreateBackend は指定エンジンのバックエンドを返します。
// 未起動の場合は遅延起動します。
func (r *RDSService) getOrCreateBackend(ctx context.Context, engine string) (*rdb.RDBBackend, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if b, ok := r.backends[engine]; ok {
		return b, nil
	}

	var b *rdb.RDBBackend
	switch engine {
	case "postgres":
		b = rdb.NewRDBBackend(&rdb.PostgreSQLEngine{}, r.logger)
		if err := b.Init(ctx, r.deps); err != nil {
			return nil, fmt.Errorf("rds: init postgres backend: %w", err)
		}
		if r.deps.Registry != nil {
			r.deps.Registry.SharedBackend("rdb-postgres-host", b.Host())
			r.deps.Registry.SharedBackend("rdb-postgres-port", b.Port())
			r.deps.Registry.SharedBackend("rdb-postgres-password", b.RootPassword())
		}
	default:
		return nil, fmt.Errorf("rds: unsupported engine: %s", engine)
	}

	r.backends[engine] = b
	return b, nil
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

// Health はサービスのヘルスステータスを返します。全バックエンドの健全性を集約します。
func (r *RDSService) Health(ctx context.Context) service.HealthStatus {
	r.mu.Lock()
	backends := make(map[string]*rdb.RDBBackend, len(r.backends))
	for k, v := range r.backends {
		backends[k] = v
	}
	r.mu.Unlock()

	for engine, b := range backends {
		status := b.Health(ctx)
		if !status.Healthy {
			return service.HealthStatus{
				Healthy: false,
				Message: fmt.Sprintf("engine %s: %s", engine, status.Message),
			}
		}
	}
	return service.HealthStatus{Healthy: true, Message: "ok"}
}

// Shutdown は全バックエンドのコンテナを停止します。
func (r *RDSService) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	backends := make(map[string]*rdb.RDBBackend, len(r.backends))
	for k, v := range r.backends {
		backends[k] = v
	}
	r.mu.Unlock()

	var firstErr error
	for engine, b := range backends {
		if err := b.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("rds: shutdown %s backend: %w", engine, err)
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
		if engine == "postgres" {
			endpointPort = 5432
		} else {
			endpointPort = 3306
		}
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
