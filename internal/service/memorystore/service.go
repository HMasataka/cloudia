package memorystore

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/service"
)

// MemorystoreService は GCP Memorystore サービスのエミュレーションを行います。
// ElastiCache が管理する Redis コンテナをバックエンドとして使用します。
type MemorystoreService struct {
	store     service.Store
	logger    *zap.Logger
	redisHost string
	redisPort string
}

// NewMemorystoreService は新しい MemorystoreService を返します。
func NewMemorystoreService(logger *zap.Logger) *MemorystoreService {
	return &MemorystoreService{
		logger: logger,
	}
}

// Name はサービス名を返します。
func (m *MemorystoreService) Name() string {
	return "memorystore"
}

// Provider はプロバイダ名を返します。
func (m *MemorystoreService) Provider() string {
	return "gcp"
}

// Init はサービスを初期化します。SharedBackend から Redis の接続情報を取得します。
func (m *MemorystoreService) Init(_ context.Context, deps service.ServiceDeps) error {
	m.store = deps.Store

	if deps.Registry == nil {
		return fmt.Errorf("memorystore: registry is nil; ElastiCache service must be registered before Memorystore")
	}

	rawHost := deps.Registry.SharedBackend("redis-host")
	if rawHost == nil {
		return fmt.Errorf("memorystore: shared backend \"redis-host\" not found; ElastiCache service must be initialized before Memorystore")
	}
	host, ok := rawHost.(string)
	if !ok || host == "" {
		return fmt.Errorf("memorystore: shared backend \"redis-host\" is not a valid string")
	}

	rawPort := deps.Registry.SharedBackend("redis-port")
	if rawPort == nil {
		return fmt.Errorf("memorystore: shared backend \"redis-port\" not found; ElastiCache service must be initialized before Memorystore")
	}
	port, ok := rawPort.(string)
	if !ok || port == "" {
		return fmt.Errorf("memorystore: shared backend \"redis-port\" is not a valid string")
	}

	m.redisHost = host
	m.redisPort = port

	return nil
}

// SupportedActions はこのサービスがサポートするアクション名の一覧を返します。
// Memorystore はパスベースのルーティングを使うため、空スライスを返します。
func (m *MemorystoreService) SupportedActions() []string {
	return []string{}
}

// Health はサービスのヘルスステータスを返します。Redis への TCP PING で確認します。
func (m *MemorystoreService) Health(ctx context.Context) service.HealthStatus {
	if m.redisHost == "" || m.redisPort == "" {
		return service.HealthStatus{Healthy: false, Message: "not initialized"}
	}

	addr := net.JoinHostPort(m.redisHost, m.redisPort)
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return service.HealthStatus{Healthy: false, Message: err.Error()}
	}
	defer conn.Close()

	if _, err := fmt.Fprint(conn, "*1\r\n$4\r\nPING\r\n"); err != nil {
		return service.HealthStatus{Healthy: false, Message: err.Error()}
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return service.HealthStatus{Healthy: false, Message: err.Error()}
	}

	line = strings.TrimSpace(line)
	if line != "+PONG" {
		return service.HealthStatus{Healthy: false, Message: fmt.Sprintf("unexpected redis response: %q", line)}
	}

	return service.HealthStatus{Healthy: true, Message: "ok"}
}

// Shutdown は no-op です。Redis コンテナは ElastiCache が管理します。
func (m *MemorystoreService) Shutdown(_ context.Context) error {
	return nil
}

// HandleRequest はアクションに応じてリクエストを処理します。
// HTTP Method + パスベースでアクション分岐します。
func (m *MemorystoreService) HandleRequest(ctx context.Context, req service.Request) (service.Response, error) {
	project, location, name, parseErr := parseMemorystorePath(req.Action)
	if parseErr != nil {
		return memorystoreErrorResponse(http.StatusBadRequest, parseErr.Error())
	}

	switch {
	case req.Method == http.MethodPost && name == "":
		// POST instances -> create
		return m.createInstance(ctx, req, project, location)
	case req.Method == http.MethodGet && name != "":
		// GET instances/{name} -> get
		return m.getInstance(ctx, project, location, name)
	case req.Method == http.MethodGet && name == "":
		// GET instances -> list
		return m.listInstances(ctx, project, location)
	case req.Method == http.MethodDelete && name != "":
		// DELETE instances/{name} -> delete
		return m.deleteInstance(ctx, project, location, name)
	case req.Method == http.MethodPatch && name != "":
		// PATCH instances/{name} -> update
		return m.updateInstance(ctx, req, project, location, name)
	default:
		return memorystoreErrorResponse(http.StatusBadRequest,
			fmt.Sprintf("unsupported method %s for path %s", req.Method, req.Action))
	}
}

// parseMemorystorePath は `projects/{p}/locations/{l}/instances[/{name}]` をパースします。
// リソースパスは /v1/ プレフィックスが除去された後の文字列です。
func parseMemorystorePath(resourcePath string) (project, location, name string, err error) {
	// パスは "projects/{p}/locations/{l}/instances" or "projects/{p}/locations/{l}/instances/{name}"
	parts := strings.Split(resourcePath, "/")
	// parts[0]="projects", parts[1]={p}, parts[2]="locations", parts[3]={l}, parts[4]="instances", [parts[5]={name}]
	if len(parts) < 5 {
		return "", "", "", fmt.Errorf("invalid memorystore path: %q", resourcePath)
	}
	if parts[0] != "projects" || parts[2] != "locations" || parts[4] != "instances" {
		return "", "", "", fmt.Errorf("invalid memorystore path: %q", resourcePath)
	}

	project = parts[1]
	location = parts[3]

	if len(parts) >= 6 {
		name = parts[5]
	}

	return project, location, name, nil
}

// memorystoreErrorResponse は GCP 互換の JSON エラーレスポンスを返します。
func memorystoreErrorResponse(statusCode int, message string) (service.Response, error) {
	grpcStatus := memorystoreGRPCStatusFromHTTP(statusCode)
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

// memorystoreGRPCStatusFromHTTP は HTTP ステータスコードを gRPC ステータス文字列に変換します。
func memorystoreGRPCStatusFromHTTP(code int) string {
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
