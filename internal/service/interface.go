package service

import "context"

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

// ServiceDeps はサービスの初期化に必要な依存関係を保持します。
// 循環依存を避けるため、各フィールドは any で受けます。
// 後続マイルストーンで具体的な型に置き換えます。
type ServiceDeps struct {
	// Store は state.Store を実装した値を受けます。
	Store any
	// LockManager は state.LockManager を受けます。
	LockManager any
	// Limiter は resource.Limiter を受けます。
	Limiter any
	// PortManager は resource.PortManager を受けます。
	PortManager any
	// DockerClient は docker.Client を受けます。
	DockerClient any
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
