package elasticache

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/config"
	awsprot "github.com/HMasataka/cloudia/internal/protocol/aws"
	"github.com/HMasataka/cloudia/internal/backend/redis"
	"github.com/HMasataka/cloudia/internal/service"
)

// ElastiCacheService は AWS ElastiCache サービスのエミュレーションを行います。
// Redis コンテナをバックエンドとして使用します。
type ElastiCacheService struct {
	redis  *redis.RedisBackend
	store  service.Store
	deps   service.ServiceDeps
	cfg    config.AWSAuthConfig
	logger *zap.Logger
}

// NewElastiCacheService は新しい ElastiCacheService を返します。
func NewElastiCacheService(cfg config.AWSAuthConfig, logger *zap.Logger) *ElastiCacheService {
	return &ElastiCacheService{
		redis:  redis.NewRedisBackend(logger),
		cfg:    cfg,
		logger: logger,
	}
}

// Name はサービス名を返します。
func (e *ElastiCacheService) Name() string {
	return "elasticache"
}

// Provider はプロバイダ名を返します。
func (e *ElastiCacheService) Provider() string {
	return "aws"
}

// Init はサービスを初期化します。Redis コンテナを起動し SharedBackend に登録します。
func (e *ElastiCacheService) Init(ctx context.Context, deps service.ServiceDeps) error {
	e.store = deps.Store
	e.deps = deps

	if err := e.redis.Init(ctx, deps); err != nil {
		return fmt.Errorf("elasticache: init redis backend: %w", err)
	}

	if deps.Registry != nil {
		deps.Registry.SharedBackend("redis-host", e.redis.Host())
		deps.Registry.SharedBackend("redis-port", e.redis.Port())
	}

	return nil
}

// HandleRequest はアクションに応じてリクエストを処理します。
func (e *ElastiCacheService) HandleRequest(ctx context.Context, req service.Request) (service.Response, error) {
	switch req.Action {
	case "CreateCacheCluster":
		return e.createCacheCluster(ctx, req)
	case "DeleteCacheCluster":
		return e.deleteCacheCluster(ctx, req)
	case "DescribeCacheClusters":
		return e.describeCacheClusters(ctx, req)
	case "ModifyCacheCluster":
		return e.modifyCacheCluster(ctx, req)
	case "CreateReplicationGroup":
		return e.createReplicationGroup(ctx, req)
	case "DeleteReplicationGroup":
		return e.deleteReplicationGroup(ctx, req)
	case "DescribeReplicationGroups":
		return e.describeReplicationGroups(ctx, req)
	default:
		return errorResponse(http.StatusBadRequest, "UnsupportedOperation",
			fmt.Sprintf("The action %q is not supported by this service.", req.Action))
	}
}

// SupportedActions はこのサービスがサポートするアクション名の一覧を返します。
func (e *ElastiCacheService) SupportedActions() []string {
	return []string{
		"CreateCacheCluster",
		"DeleteCacheCluster",
		"DescribeCacheClusters",
		"ModifyCacheCluster",
		"CreateReplicationGroup",
		"DeleteReplicationGroup",
		"DescribeReplicationGroups",
	}
}

// Health はサービスのヘルスステータスを返します。
func (e *ElastiCacheService) Health(ctx context.Context) service.HealthStatus {
	return e.redis.Health(ctx)
}

// Shutdown は Redis コンテナを停止・削除します。
func (e *ElastiCacheService) Shutdown(ctx context.Context) error {
	return e.redis.Shutdown(ctx, e.deps)
}

// errorResponse は AWS 互換の XML エラーレスポンスを返します。
func errorResponse(statusCode int, code, message string) (service.Response, error) {
	resp, err := awsprot.MarshalXMLResponse(statusCode, awsprot.ErrorResponse{
		Error: awsprot.ErrorDetail{
			Code:    code,
			Message: message,
		},
		RequestID: "cloudia-elasticache",
	}, xmlNamespace)
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}
	return resp, nil
}

// redisPort はバックエンドの Redis ポートを整数で返します。
func (e *ElastiCacheService) redisPort() int {
	p, _ := strconv.Atoi(e.redis.Port())
	return p
}
