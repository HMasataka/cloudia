package gce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/state"
)

// GCEService は GCP Compute Engine サービスのエミュレーションを行います。
// Docker コンテナを GCE インスタンスのバックエンドとして使用します。
type GCEService struct {
	store       service.Store
	docker      service.ContainerRunner
	lockManager service.LockManager
	limiter     service.Limiter
	logger      *zap.Logger
}

// NewGCEService は新しい GCEService を返します。
func NewGCEService(logger *zap.Logger) *GCEService {
	return &GCEService{
		logger: logger,
	}
}

// Name はサービス名を返します。
func (g *GCEService) Name() string {
	return "compute"
}

// Provider はプロバイダ名を返します。
func (g *GCEService) Provider() string {
	return "gcp"
}

// Init はサービスを初期化します。
func (g *GCEService) Init(_ context.Context, deps service.ServiceDeps) error {
	g.store = deps.Store
	g.docker = deps.DockerClient
	g.lockManager = deps.LockManager
	g.limiter = deps.Limiter
	return nil
}

// SupportedActions はこのサービスがサポートするアクション名の一覧を返します。
// GCE はパスベースのルーティングを使うため、空スライスを返します。
func (g *GCEService) SupportedActions() []string {
	return []string{}
}

// Health はサービスのヘルスステータスを返します。
func (g *GCEService) Health(_ context.Context) service.HealthStatus {
	return service.HealthStatus{Healthy: true, Message: "ok"}
}

// Shutdown は管理中のコンテナを全て停止・削除します。
func (g *GCEService) Shutdown(ctx context.Context) error {
	if g.store == nil || g.docker == nil {
		return nil
	}

	instances, err := g.store.List(ctx, kindInstance, state.Filter{})
	if err != nil {
		return fmt.Errorf("gce shutdown: list instances: %w", err)
	}

	var firstErr error
	for _, r := range instances {
		containerID := r.ContainerID
		if containerID == "" {
			continue
		}
		if stopErr := g.docker.StopContainer(ctx, containerID, nil); stopErr != nil {
			g.logger.Warn("gce shutdown: stop container failed",
				zap.String("instance_name", r.ID),
				zap.String("container_id", containerID),
				zap.Error(stopErr),
			)
			if firstErr == nil {
				firstErr = stopErr
			}
		}
		if rmErr := g.docker.RemoveContainer(ctx, containerID); rmErr != nil {
			g.logger.Warn("gce shutdown: remove container failed",
				zap.String("instance_name", r.ID),
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

// HandleRequest はアクションに応じてリクエストを処理します。
// GCE は req.Action にリソースパス、req.Method に HTTP メソッドを使います。
func (g *GCEService) HandleRequest(ctx context.Context, req service.Request) (service.Response, error) {
	project, zone, name, op, parseErr := parseComputePath(req.Action)
	if parseErr != nil {
		return gceErrorResponse(http.StatusBadRequest, parseErr.Error())
	}

	switch {
	case req.Method == http.MethodPost && name == "" && op == "":
		// POST instances -> insert
		return g.insertInstance(ctx, req, project, zone)
	case req.Method == http.MethodGet && name != "" && op == "":
		// GET instances/{name} -> get
		return g.getInstance(ctx, project, zone, name)
	case req.Method == http.MethodGet && name == "" && op == "":
		// GET instances -> list
		return g.listInstances(ctx, project, zone)
	case req.Method == http.MethodDelete && name != "" && op == "":
		// DELETE instances/{name} -> delete
		return g.deleteInstance(ctx, project, zone, name)
	case req.Method == http.MethodPost && name != "" && op == "start":
		// POST instances/{name}/start -> start
		return g.startInstance(ctx, project, zone, name)
	case req.Method == http.MethodPost && name != "" && op == "stop":
		// POST instances/{name}/stop -> stop
		return g.stopInstance(ctx, project, zone, name)
	default:
		return gceErrorResponse(http.StatusBadRequest,
			fmt.Sprintf("unsupported method %s for path %s", req.Method, req.Action))
	}
}

// parseComputePath は `projects/{p}/zones/{z}/instances[/{name}[/{op}]]` をパースします。
// リソースパスは /compute/v1/ プレフィックスが除去された後の文字列です。
func parseComputePath(resourcePath string) (project, zone, name, op string, err error) {
	// パスは "projects/{p}/zones/{z}/instances" or "projects/{p}/zones/{z}/instances/{name}" or "projects/{p}/zones/{z}/instances/{name}/{op}"
	parts := strings.SplitN(resourcePath, "/", -1)
	// parts[0]="projects", parts[1]={p}, parts[2]="zones", parts[3]={z}, parts[4]="instances", [parts[5]={name}], [parts[6]={op}]
	if len(parts) < 5 {
		return "", "", "", "", fmt.Errorf("invalid compute path: %q", resourcePath)
	}
	if parts[0] != "projects" || parts[2] != "zones" || parts[4] != "instances" {
		return "", "", "", "", fmt.Errorf("invalid compute path: %q", resourcePath)
	}

	project = parts[1]
	zone = parts[3]

	if len(parts) >= 6 {
		name = parts[5]
	}
	if len(parts) >= 7 {
		op = parts[6]
	}

	return project, zone, name, op, nil
}

// gceErrorResponse は GCP 互換の JSON エラーレスポンスを返します。
func gceErrorResponse(statusCode int, message string) (service.Response, error) {
	grpcStatus := grpcStatusFromHTTP(statusCode)
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

// grpcStatusFromHTTP は HTTP ステータスコードを gRPC ステータス文字列に変換します。
func grpcStatusFromHTTP(code int) string {
	switch code {
	case http.StatusBadRequest:
		return "INVALID_ARGUMENT"
	case http.StatusUnauthorized:
		return "UNAUTHENTICATED"
	case http.StatusForbidden:
		return "PERMISSION_DENIED"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusConflict:
		return "ALREADY_EXISTS"
	case http.StatusNotImplemented:
		return "UNIMPLEMENTED"
	default:
		return "UNKNOWN"
	}
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
