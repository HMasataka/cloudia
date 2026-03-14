package gke

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/backend/k3s"
	"github.com/HMasataka/cloudia/internal/service"
)

// defaultBackendFactory は本番用の K3sBackend を生成するファクトリです。
func defaultBackendFactory(logger *zap.Logger) ClusterBackend {
	return k3s.NewK3sBackend(logger)
}

// ClusterBackend は k3s クラスタバックエンドのインターフェースです。
type ClusterBackend interface {
	Start(ctx context.Context, deps service.ServiceDeps, clusterName string) error
	Kubeconfig() string
	Endpoint() string
	CertificateAuthority() string
	ContainerID() string
	Shutdown(ctx context.Context) error
}

// ClusterBackendFactory は ClusterBackend を生成するファクトリ関数の型です。
type ClusterBackendFactory func(logger *zap.Logger) ClusterBackend

// GKEService は GCP Kubernetes Engine サービスのエミュレーションを行います。
// k3s コンテナを GKE クラスタのバックエンドとして使用します。
type GKEService struct {
	store          service.Store
	deps           service.ServiceDeps
	logger         *zap.Logger
	backendFactory ClusterBackendFactory
}

// NewGKEService は新しい GKEService を返します。
func NewGKEService(logger *zap.Logger) *GKEService {
	return &GKEService{
		logger:         logger,
		backendFactory: defaultBackendFactory,
	}
}

// WithBackendFactory はテスト用にバックエンドファクトリを差し替えます。
func (g *GKEService) WithBackendFactory(f ClusterBackendFactory) *GKEService {
	g.backendFactory = f
	return g
}

// Name はサービス名を返します。
func (g *GKEService) Name() string {
	return "gke"
}

// Provider はプロバイダ名を返します。
func (g *GKEService) Provider() string {
	return "gcp"
}

// Init はサービスを初期化します。
func (g *GKEService) Init(_ context.Context, deps service.ServiceDeps) error {
	g.store = deps.Store
	g.deps = deps
	return nil
}

// SupportedActions はこのサービスがサポートするアクション名の一覧を返します。
// GKE はパスベースのルーティングを使うため、空スライスを返します。
func (g *GKEService) SupportedActions() []string {
	return []string{}
}

// Health はサービスのヘルスステータスを返します。
func (g *GKEService) Health(_ context.Context) service.HealthStatus {
	return service.HealthStatus{Healthy: true, Message: "ok"}
}

// Shutdown は管理中のクラスタを全て停止・削除します。
func (g *GKEService) Shutdown(_ context.Context) error {
	return nil
}

// HandleRequest はアクションに応じてリクエストを処理します。
// GKE は req.Action にリソースパス、req.Method に HTTP メソッドを使います。
func (g *GKEService) HandleRequest(ctx context.Context, req service.Request) (service.Response, error) {
	project, location, name, parseErr := parseGKEPath(req.Action)
	if parseErr != nil {
		return gkeErrorResponse(http.StatusBadRequest, parseErr.Error(), "INVALID_ARGUMENT")
	}

	switch {
	case req.Method == http.MethodPost && name == "":
		// POST clusters -> create
		return g.createCluster(ctx, req, project, location)
	case req.Method == http.MethodGet && name != "":
		// GET clusters/{name} -> get
		return g.getCluster(ctx, project, location, name)
	case req.Method == http.MethodGet && name == "":
		// GET clusters -> list
		return g.listClusters(ctx, project, location)
	case req.Method == http.MethodDelete && name != "":
		// DELETE clusters/{name} -> delete
		return g.deleteCluster(ctx, project, location, name)
	default:
		return gkeErrorResponse(http.StatusBadRequest,
			fmt.Sprintf("unsupported method %s for path %s", req.Method, req.Action),
			"INVALID_ARGUMENT")
	}
}

// parseGKEPath は `projects/{p}/locations/{l}/clusters[/{name}]` をパースします。
// リソースパスは /v1/ プレフィックスが除去された後の文字列です。
func parseGKEPath(resourcePath string) (project, location, name string, err error) {
	// パスは "projects/{p}/locations/{l}/clusters" or "projects/{p}/locations/{l}/clusters/{name}"
	parts := strings.Split(resourcePath, "/")
	// parts[0]="projects", parts[1]={p}, parts[2]="locations", parts[3]={l}, parts[4]="clusters", [parts[5]={name}]
	if len(parts) < 5 {
		return "", "", "", fmt.Errorf("invalid gke path: %q", resourcePath)
	}
	if parts[0] != "projects" || parts[2] != "locations" || parts[4] != "clusters" {
		return "", "", "", fmt.Errorf("invalid gke path: %q", resourcePath)
	}

	project = parts[1]
	location = parts[3]

	if len(parts) >= 6 {
		name = parts[5]
	}

	return project, location, name, nil
}

// gkeErrorResponse は GCP 互換の JSON エラーレスポンスを返します。
func gkeErrorResponse(statusCode int, message, grpcStatus string) (service.Response, error) {
	body, marshalErr := json.Marshal(map[string]interface{}{
		"error": map[string]interface{}{
			"code":    statusCode,
			"message": message,
			"status":  grpcStatus,
		},
	})
	if marshalErr != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, marshalErr
	}
	return service.Response{
		StatusCode:  statusCode,
		Body:        body,
		ContentType: "application/json; charset=utf-8",
	}, nil
}

// jsonResponse は JSON レスポンスを構築します。
func jsonResponse(statusCode int, body interface{}) (service.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}
	return service.Response{
		StatusCode:  statusCode,
		Body:        b,
		ContentType: "application/json; charset=utf-8",
	}, nil
}
