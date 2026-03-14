package eks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/service"
)

const (
	kindCluster  = "aws:eks:cluster"
	awsPartition = "aws"
	awsRegion    = "us-east-1"
	awsAccountID = "000000000000"
	contentType  = "application/json"
)

// ClusterBackend は k3s クラスタのライフサイクルを管理するインターフェースです。
type ClusterBackend interface {
	Start(ctx context.Context, deps service.ServiceDeps, clusterName string) error
	Endpoint() string
	Kubeconfig() string
	CertificateAuthority() string
	ContainerID() string
	Shutdown(ctx context.Context) error
}

// BackendFactory は ClusterBackend を生成するファクトリ関数型です。
type BackendFactory func(logger *zap.Logger) ClusterBackend

// EKSService は AWS EKS サービスのエミュレーションを行います。
// k3s コンテナを EKS クラスタのバックエンドとして使用します。
type EKSService struct {
	store          service.Store
	deps           service.ServiceDeps
	logger         *zap.Logger
	backendFactory BackendFactory
	mu             sync.Mutex
	backends       map[string]ClusterBackend
}

// NewEKSService は新しい EKSService を返します。
func NewEKSService(logger *zap.Logger) *EKSService {
	return &EKSService{
		logger:         logger,
		backendFactory: defaultBackendFactory,
		backends:       make(map[string]ClusterBackend),
	}
}

// SetBackendFactory はテスト用に ClusterBackend ファクトリを差し替えます。
func (s *EKSService) SetBackendFactory(f BackendFactory) {
	s.backendFactory = f
}

// Name はサービス名を返します。
func (s *EKSService) Name() string {
	return "eks"
}

// Provider はプロバイダ名を返します。
func (s *EKSService) Provider() string {
	return "aws"
}

// Init はサービスを初期化します。
func (s *EKSService) Init(_ context.Context, deps service.ServiceDeps) error {
	s.store = deps.Store
	s.deps = deps
	return nil
}

// HandleRequest はアクションに応じてリクエストを処理します。
// Method + Action のパターンマッチで分岐します。
func (s *EKSService) HandleRequest(ctx context.Context, req service.Request) (service.Response, error) {
	method := req.Method
	action := req.Action

	switch {
	case method == http.MethodPost && action == "clusters":
		return s.createCluster(ctx, req)
	case method == http.MethodGet && action == "clusters":
		return s.listClusters(ctx, req)
	case method == http.MethodGet && strings.HasPrefix(action, "clusters/"):
		return s.describeCluster(ctx, req)
	case method == http.MethodDelete && strings.HasPrefix(action, "clusters/"):
		return s.deleteCluster(ctx, req)
	default:
		return jsonErrorResponse(http.StatusBadRequest, "UnsupportedOperation",
			fmt.Sprintf("The action %q with method %q is not supported.", action, method))
	}
}

// SupportedActions はこのサービスがサポートするアクション名の一覧を返します。
func (s *EKSService) SupportedActions() []string {
	return []string{
		"createCluster",
		"describeCluster",
		"deleteCluster",
		"listClusters",
	}
}

// Health はサービスのヘルスステータスを返します。
func (s *EKSService) Health(_ context.Context) service.HealthStatus {
	return service.HealthStatus{Healthy: true, Message: "ok"}
}

// Shutdown は管理中の全クラスタバックエンドを停止します。
func (s *EKSService) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var firstErr error
	for name, b := range s.backends {
		if err := b.Shutdown(ctx); err != nil {
			s.logger.Warn("eks: shutdown backend failed",
				zap.String("cluster", name),
				zap.Error(err),
			)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// clusterNameFromAction は "clusters/{name}" 形式の action から name を抽出します。
func clusterNameFromAction(action string) string {
	return strings.TrimPrefix(action, "clusters/")
}

// clusterARN は EKS クラスタの ARN を生成します。
func clusterARN(name string) string {
	return fmt.Sprintf("arn:%s:eks:%s:%s:cluster/%s", awsPartition, awsRegion, awsAccountID, name)
}

// jsonErrorResponse は JSON 形式のエラーレスポンスを返します。
func jsonErrorResponse(statusCode int, errType, message string) (service.Response, error) {
	body, err := json.Marshal(eksError{Message: message, Type: errType})
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}
	return service.Response{
		StatusCode:  statusCode,
		Body:        body,
		ContentType: contentType,
	}, nil
}

// jsonResponse は JSON 形式のレスポンスを返します。
func jsonResponse(statusCode int, v interface{}) (service.Response, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}
	return service.Response{
		StatusCode:  statusCode,
		Body:        body,
		ContentType: contentType,
	}, nil
}
