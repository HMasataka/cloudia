package lambda

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sync"

	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/pkg/models"
	"go.uber.org/zap"
)

const (
	resourceKind    = "aws:lambda:function"
	lambdaNetwork   = "cloudia"
	contentType     = "application/json"
	lambdaCodeDir   = "/tmp/cloudia/lambda/functions"
)

// containerEntry はインメモリのコンテナプールエントリです。
// Group 3 (Invoke) で利用します。
type containerEntry struct {
	containerID string
	hostPort    int
	baseURL     string
	status      string // "starting", "ready", "stopping"
}

// LambdaService は service.Service と service.ProxyService を実装します。
type LambdaService struct {
	cfg    config.AWSAuthConfig
	store  service.Store
	docker service.ContainerRunner
	ports  service.PortAllocator
	limits service.Limiter
	logger *zap.Logger

	// containerPool は関数名 → コンテナエントリのインメモリマップです (Group 3 用)。
	poolMu    sync.Mutex
	pool      map[string]*containerEntry
}

// NewLambdaService は新しい LambdaService を返します。
func NewLambdaService(cfg config.AWSAuthConfig, logger *zap.Logger) *LambdaService {
	return &LambdaService{
		cfg:    cfg,
		logger: logger,
		pool:   make(map[string]*containerEntry),
	}
}

// Name はサービス名を返します。
func (s *LambdaService) Name() string {
	return "lambda"
}

// Provider はプロバイダ名を返します。
func (s *LambdaService) Provider() string {
	return "aws"
}

// Init はサービスを初期化します。
func (s *LambdaService) Init(_ context.Context, deps service.ServiceDeps) error {
	s.store = deps.Store
	s.docker = deps.DockerClient
	s.ports = deps.PortAllocator
	s.limits = deps.Limiter
	return nil
}

// HandleRequest は ProxyService として ServeHTTP に委譲されるため、常に ErrUnsupportedOperation を返します。
func (s *LambdaService) HandleRequest(_ context.Context, _ service.Request) (service.Response, error) {
	return service.Response{}, models.ErrUnsupportedOperation
}

// SupportedActions は Lambda がサポートするアクション一覧を返します。
func (s *LambdaService) SupportedActions() []string {
	return []string{
		"CreateFunction",
		"GetFunction",
		"DeleteFunction",
		"ListFunctions",
		"UpdateFunctionCode",
		"Invoke",
	}
}

// Health は常に healthy を返します。
func (s *LambdaService) Health(_ context.Context) service.HealthStatus {
	return service.HealthStatus{Healthy: true, Message: "ok"}
}

// Shutdown は起動中の全 Lambda コンテナを停止・削除し、一時ファイルをクリーンアップします。
func (s *LambdaService) Shutdown(ctx context.Context) error {
	s.poolMu.Lock()
	defer s.poolMu.Unlock()

	for name, entry := range s.pool {
		if entry.containerID == "" {
			continue
		}
		if err := s.docker.StopContainer(ctx, entry.containerID, nil); err != nil {
			s.logger.Warn("lambda: stop container failed",
				zap.String("function", name),
				zap.Error(err),
			)
		}
		if err := s.docker.RemoveContainer(ctx, entry.containerID); err != nil {
			s.logger.Warn("lambda: remove container failed",
				zap.String("function", name),
				zap.Error(err),
			)
		}
		if entry.hostPort != 0 {
			s.ports.Release(entry.hostPort)
		}
	}
	s.pool = make(map[string]*containerEntry)

	if err := os.RemoveAll(lambdaCodeDir); err != nil {
		s.logger.Warn("lambda: remove code dir failed", zap.Error(err))
	}

	return nil
}

// writeError は Lambda エラーレスポンスを書き込みます。
// フォーマット: {"__type":"ExceptionType","Message":"..."}
func writeError(w http.ResponseWriter, statusCode int, errType, message string) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(statusCode)
	b, _ := json.Marshal(lambdaError{Type: errType, Message: message})
	w.Write(b) //nolint:errcheck
}

// writeJSON は JSON レスポンスを書き込みます。
func writeJSON(w http.ResponseWriter, statusCode int, body any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(statusCode)
	b, err := json.Marshal(body)
	if err != nil {
		return
	}
	w.Write(b) //nolint:errcheck
}
