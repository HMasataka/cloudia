package service

import (
	"context"

	"github.com/HMasataka/cloudia/internal/backend/docker"
	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
)

// HealthStatus はサービスのヘルスチェック結果を表します。
type HealthStatus struct {
	Healthy bool
	Message string
}

// Request はサービスへのリクエストを表します。
type Request struct {
	Provider string
	Service  string
	Action   string
	Params   map[string]string
	Body     []byte
	Headers  map[string]string
}

// Response はサービスからのレスポンスを表します。
type Response struct {
	StatusCode  int
	Body        []byte
	ContentType string
	Headers     map[string]string
}

// Store はリソースの永続化操作を定義します。
type Store interface {
	Get(ctx context.Context, kind, id string) (*models.Resource, error)
	List(ctx context.Context, kind string, filter state.Filter) ([]*models.Resource, error)
	Put(ctx context.Context, resource *models.Resource) error
	Delete(ctx context.Context, kind, id string) error
}

// LockManager はリソースごとの排他ロックを定義します。
type LockManager interface {
	Lock(ctx context.Context, kind, id string) error
	Unlock(kind, id string)
}

// Limiter はリソース制限チェックを定義します。
type Limiter interface {
	CheckContainerLimit(ctx context.Context) error
}

// PortAllocator はポートの割り当てと解放を定義します。
type PortAllocator interface {
	Allocate(preferred int, resourceID string) (int, error)
	Release(port int)
}

// ContainerRunner は Docker コンテナの操作を定義します。
type ContainerRunner interface {
	RunContainer(ctx context.Context, cfg docker.ContainerConfig) (string, error)
	StopContainer(ctx context.Context, containerID string, timeout *int) error
	RemoveContainer(ctx context.Context, containerID string) error
}

// NetworkManager は Docker ネットワークの操作を定義します。
type NetworkManager interface {
	CreateNetwork(ctx context.Context, name, cidr string) (string, error)
	RemoveNetwork(ctx context.Context, networkID string) error
}

// ServiceDeps はサービスの初期化に必要な依存関係を保持します。
type ServiceDeps struct {
	Store          Store
	LockManager    LockManager
	Limiter        Limiter
	PortAllocator  PortAllocator
	DockerClient   ContainerRunner
	NetworkManager NetworkManager
	Registry       *Registry
}

// Service はクラウドサービスプロバイダが実装すべきインターフェースです。
type Service interface {
	// Name はサービス名を返します (例: "s3", "compute")。
	Name() string

	// Provider はプロバイダ名を返します (例: "aws", "gcp")。
	Provider() string

	// Init はサービスを初期化します。
	Init(ctx context.Context, deps ServiceDeps) error

	// HandleRequest はリクエストを処理してレスポンスを返します。
	HandleRequest(ctx context.Context, req Request) (Response, error)

	// SupportedActions はこのサービスがサポートするアクション名の一覧を返します。
	SupportedActions() []string

	// Health はサービスのヘルスステータスを返します。
	Health(ctx context.Context) HealthStatus

	// Shutdown はサービスをシャットダウンします。
	Shutdown(ctx context.Context) error
}
