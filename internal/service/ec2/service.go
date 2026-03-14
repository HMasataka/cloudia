package ec2

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"

	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/config"
	awsprot "github.com/HMasataka/cloudia/internal/protocol/aws"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/state"
)

// EC2Service は AWS EC2 サービスのエミュレーションを行います。
// Docker コンテナを EC2 インスタンスのバックエンドとして使用します。
type EC2Service struct {
	cfg         config.AWSAuthConfig
	store       service.Store
	docker      service.ContainerRunner
	lockManager service.LockManager
	limiter     service.Limiter
	logger      *zap.Logger
}

// NewEC2Service は新しい EC2Service を返します。
func NewEC2Service(cfg config.AWSAuthConfig, logger *zap.Logger) *EC2Service {
	return &EC2Service{
		cfg:    cfg,
		logger: logger,
	}
}

// Name はサービス名を返します。
func (e *EC2Service) Name() string {
	return "ec2"
}

// Provider はプロバイダ名を返します。
func (e *EC2Service) Provider() string {
	return "aws"
}

// Init はサービスを初期化します。
func (e *EC2Service) Init(_ context.Context, deps service.ServiceDeps) error {
	e.store = deps.Store
	e.docker = deps.DockerClient
	e.lockManager = deps.LockManager
	e.limiter = deps.Limiter
	return nil
}

// HandleRequest はアクションに応じてリクエストを処理します。
func (e *EC2Service) HandleRequest(ctx context.Context, req service.Request) (service.Response, error) {
	switch req.Action {
	case "RunInstances":
		return e.runInstances(ctx, req)
	case "TerminateInstances":
		return e.terminateInstances(ctx, req)
	case "DescribeInstances":
		return e.describeInstances(ctx, req)
	case "StartInstances":
		return e.startInstances(ctx, req)
	case "StopInstances":
		return e.stopInstances(ctx, req)
	case "CreateTags":
		return e.createTags(ctx, req)
	case "DeleteTags":
		return e.deleteTags(ctx, req)
	case "DescribeTags":
		return e.describeTags(ctx, req)
	case "CreateKeyPair":
		return e.createKeyPair(ctx, req)
	case "ImportKeyPair":
		return e.importKeyPair(ctx, req)
	case "DeleteKeyPair":
		return e.deleteKeyPair(ctx, req)
	case "DescribeKeyPairs":
		return e.describeKeyPairs(ctx, req)
	default:
		return errorResponse(http.StatusBadRequest, "UnsupportedOperation",
			fmt.Sprintf("The action %q is not supported by this service.", req.Action))
	}
}

// SupportedActions はこのサービスがサポートするアクション名の一覧を返します。
func (e *EC2Service) SupportedActions() []string {
	return []string{
		"RunInstances",
		"TerminateInstances",
		"DescribeInstances",
		"StartInstances",
		"StopInstances",
		"CreateTags",
		"DeleteTags",
		"DescribeTags",
		"CreateKeyPair",
		"ImportKeyPair",
		"DeleteKeyPair",
		"DescribeKeyPairs",
	}
}

// Health はサービスのヘルスステータスを返します。
func (e *EC2Service) Health(_ context.Context) service.HealthStatus {
	return service.HealthStatus{Healthy: true, Message: "ok"}
}

// Shutdown は管理中のコンテナを全て停止・削除します。
func (e *EC2Service) Shutdown(ctx context.Context) error {
	if e.store == nil || e.docker == nil {
		return nil
	}

	instances, err := e.store.List(ctx, kindInstance, state.Filter{})
	if err != nil {
		return fmt.Errorf("ec2 shutdown: list instances: %w", err)
	}

	var firstErr error
	for _, r := range instances {
		containerID := r.ContainerID
		if containerID == "" {
			continue
		}
		if stopErr := e.docker.StopContainer(ctx, containerID, nil); stopErr != nil {
			e.logger.Warn("ec2 shutdown: stop container failed",
				zap.String("instance_id", r.ID),
				zap.String("container_id", containerID),
				zap.Error(stopErr),
			)
			if firstErr == nil {
				firstErr = stopErr
			}
		}
		if rmErr := e.docker.RemoveContainer(ctx, containerID); rmErr != nil {
			e.logger.Warn("ec2 shutdown: remove container failed",
				zap.String("instance_id", r.ID),
				zap.String("container_id", containerID),
				zap.Error(rmErr),
			)
			if firstErr == nil {
				firstErr = rmErr
			}
		}
	}

	return firstErr
}

// generateHex17 は 17 文字の小文字 hex 文字列を生成します。
func generateHex17() (string, error) {
	b := make([]byte, 9)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate hex17: %w", err)
	}
	return hex.EncodeToString(b)[:17], nil
}

// errorResponse は AWS 互換の XML エラーレスポンスを返します。
func errorResponse(statusCode int, code, message string) (service.Response, error) {
	resp, err := awsprot.MarshalXMLResponse(statusCode, awsprot.ErrorResponse{
		Error: awsprot.ErrorDetail{
			Code:    code,
			Message: message,
		},
		RequestID: "cloudia-ec2",
	}, xmlNamespace)
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}
	return resp, nil
}

