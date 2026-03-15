package cloudsql

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/service"
)

// CloudSQLService は GCP Cloud SQL サービスのエミュレーションを行います。
// RDS が管理する MySQL/PostgreSQL コンテナをバックエンドとして使用します。
type CloudSQLService struct {
	store   service.Store
	logger  *zap.Logger
	dbHosts map[string]string
	dbPorts map[string]string
}

// NewCloudSQLService は新しい CloudSQLService を返します。
func NewCloudSQLService(logger *zap.Logger) *CloudSQLService {
	return &CloudSQLService{
		logger:  logger,
		dbHosts: make(map[string]string),
		dbPorts: make(map[string]string),
	}
}

// Name はサービス名を返します。
func (c *CloudSQLService) Name() string {
	return "cloudsql"
}

// Provider はプロバイダ名を返します。
func (c *CloudSQLService) Provider() string {
	return "gcp"
}

// Init はサービスを初期化します。SharedBackend から MySQL/PostgreSQL の接続情報を取得します。
// postgres バックエンドが未登録（遅延起動のため）でもエラーにしません。
func (c *CloudSQLService) Init(_ context.Context, deps service.ServiceDeps) error {
	c.store = deps.Store

	if deps.Registry == nil {
		return fmt.Errorf("cloudsql: registry is nil; RDS service must be registered before Cloud SQL")
	}

	// MySQL バックエンドの接続情報を取得（必須）。
	mysqlHost, mysqlPort, err := c.resolveBackendAddr(deps.Registry, "rdb-mysql-host", "rdb-mysql-port")
	if err != nil {
		return fmt.Errorf("cloudsql: mysql backend: %w", err)
	}
	c.dbHosts["mysql"] = mysqlHost
	c.dbPorts["mysql"] = mysqlPort

	// PostgreSQL バックエンドの接続情報を取得（任意: 遅延起動のため未登録でも許容）。
	pgHost := sharedBackendString(deps.Registry, "rdb-postgres-host")
	pgPort := sharedBackendString(deps.Registry, "rdb-postgres-port")
	if pgHost != "" && pgPort != "" {
		c.dbHosts["postgres"] = pgHost
		c.dbPorts["postgres"] = pgPort
	}

	return nil
}

// resolveBackendAddr は SharedBackend から host/port を取得します。
func (c *CloudSQLService) resolveBackendAddr(registry *service.Registry, hostKey, portKey string) (host, port string, err error) {
	host = sharedBackendString(registry, hostKey)
	if host == "" {
		return "", "", fmt.Errorf("shared backend %q not found; RDS service must be initialized before Cloud SQL", hostKey)
	}

	port = sharedBackendString(registry, portKey)
	if port == "" {
		return "", "", fmt.Errorf("shared backend %q not found; RDS service must be initialized before Cloud SQL", portKey)
	}

	return host, port, nil
}

// sharedBackendString は Registry から文字列値を取得します。未登録または非文字列の場合は空文字を返します。
func sharedBackendString(registry *service.Registry, key string) string {
	raw := registry.SharedBackend(key)
	if raw == nil {
		return ""
	}
	s, _ := raw.(string)
	return s
}

// SupportedActions はこのサービスがサポートするアクション名の一覧を返します。
// Cloud SQL はパスベースのルーティングを使うため、空スライスを返します。
func (c *CloudSQLService) SupportedActions() []string {
	return []string{}
}

// Health はサービスのヘルスステータスを返します。登録済みの全バックエンドへの TCP 接続で確認します。
// いずれかのバックエンドへの接続が失敗した場合は unhealthy を返します。
func (c *CloudSQLService) Health(ctx context.Context) service.HealthStatus {
	if len(c.dbHosts) == 0 {
		return service.HealthStatus{Healthy: false, Message: "not initialized"}
	}

	for engine, host := range c.dbHosts {
		port, ok := c.dbPorts[engine]
		if !ok || host == "" || port == "" {
			continue
		}
		addr := net.JoinHostPort(host, port)
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
		if err != nil {
			return service.HealthStatus{Healthy: false, Message: fmt.Sprintf("engine %s: %s", engine, err.Error())}
		}
		conn.Close()
	}

	return service.HealthStatus{Healthy: true, Message: "ok"}
}

// Shutdown は no-op です。MySQL コンテナは RDS が管理します。
func (c *CloudSQLService) Shutdown(_ context.Context) error {
	return nil
}

// HandleRequest はアクションに応じてリクエストを処理します。
// HTTP Method + パスベースでアクション分岐します。
func (c *CloudSQLService) HandleRequest(ctx context.Context, req service.Request) (service.Response, error) {
	project, name, parseErr := parseCloudSQLPath(req.Action)
	if parseErr != nil {
		return cloudsqlErrorResponse(http.StatusBadRequest, parseErr.Error())
	}

	switch {
	case req.Method == http.MethodPost && name == "":
		// POST instances -> insert
		return c.insertInstance(ctx, req, project)
	case req.Method == http.MethodGet && name != "":
		// GET instances/{name} -> get
		return c.getInstance(ctx, project, name)
	case req.Method == http.MethodGet && name == "":
		// GET instances -> list
		return c.listInstances(ctx, project)
	case req.Method == http.MethodDelete && name != "":
		// DELETE instances/{name} -> delete
		return c.deleteInstance(ctx, project, name)
	case req.Method == http.MethodPatch && name != "":
		// PATCH instances/{name} -> update
		return c.updateInstance(ctx, req, project, name)
	default:
		return cloudsqlErrorResponse(http.StatusBadRequest,
			fmt.Sprintf("unsupported method %s for path %s", req.Method, req.Action))
	}
}

// parseCloudSQLPath は `projects/{p}/instances[/{name}]` をパースします。
// リソースパスは /v1/ プレフィックスが除去された後の文字列です。
func parseCloudSQLPath(resourcePath string) (project, name string, err error) {
	// パスは "projects/{p}/instances" or "projects/{p}/instances/{name}"
	parts := strings.Split(resourcePath, "/")
	// parts[0]="projects", parts[1]={p}, parts[2]="instances", [parts[3]={name}]
	if len(parts) < 3 {
		return "", "", fmt.Errorf("invalid cloudsql path: %q", resourcePath)
	}
	if parts[0] != "projects" || parts[2] != "instances" {
		return "", "", fmt.Errorf("invalid cloudsql path: %q", resourcePath)
	}

	project = parts[1]

	if len(parts) >= 4 {
		name = parts[3]
	}

	return project, name, nil
}

// cloudsqlErrorResponse は GCP 互換の JSON エラーレスポンスを返します。
func cloudsqlErrorResponse(statusCode int, message string) (service.Response, error) {
	grpcStatus := cloudsqlGRPCStatusFromHTTP(statusCode)
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

// cloudsqlGRPCStatusFromHTTP は HTTP ステータスコードを gRPC ステータス文字列に変換します。
func cloudsqlGRPCStatusFromHTTP(code int) string {
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
