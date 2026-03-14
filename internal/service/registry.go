package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/HMasataka/cloudia/pkg/models"
)

// ServiceMeta はサービスのメタデータを保持します。
type ServiceMeta struct {
	// Provider はプロバイダ名です (例: "aws", "gcp")。
	Provider string
	// Name はサービス名です (例: "s3", "compute")。
	Name string
	// PathPrefixes は GCP 用のパスプレフィックス一覧です。
	PathPrefixes []string
	// TargetPrefix は AWS JSON プロトコル用のターゲットプレフィックスです。
	TargetPrefix string
	// AWSProtocol は AWS のプロトコル種別です ("query" or "json")。
	AWSProtocol string
}

// Registry はサービスの登録・解決・ライフサイクル管理を担います。
type Registry struct {
	mu       sync.RWMutex
	services map[string]Service
	order    []string        // 登録順序を保持（逆順 Shutdown のため）
	backends map[string]any  // 共有バックエンド
	metas    map[string]ServiceMeta // キーは "provider:name"
}

// NewRegistry は空の Registry を返します。
func NewRegistry() *Registry {
	return &Registry{
		services: make(map[string]Service),
		order:    []string{},
		backends: make(map[string]any),
		metas:    make(map[string]ServiceMeta),
	}
}

// Register はサービスを "provider:name" キーで登録します。
// 同一キーが既に登録されている場合は models.ErrAlreadyExists を返します。
func (r *Registry) Register(svc Service) error {
	key := fmt.Sprintf("%s:%s", svc.Provider(), svc.Name())

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.services[key]; exists {
		return fmt.Errorf("service %q: %w", key, models.ErrAlreadyExists)
	}

	r.services[key] = svc
	r.order = append(r.order, key)

	return nil
}

// RegisterWithMeta はメタデータ付きでサービスを登録します。
// 内部で Register を呼んだ後、meta を "provider:name" キーで保存します。
// 同一キーが既に登録されている場合は models.ErrAlreadyExists を返します。
func (r *Registry) RegisterWithMeta(svc Service, meta ServiceMeta) error {
	if err := r.Register(svc); err != nil {
		return err
	}

	key := fmt.Sprintf("%s:%s", svc.Provider(), svc.Name())

	r.mu.Lock()
	defer r.mu.Unlock()

	r.metas[key] = meta

	return nil
}

// MetaByProvider は指定プロバイダのメタデータ一覧を返します。
func (r *Registry) MetaByProvider(provider string) []ServiceMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []ServiceMeta
	for _, meta := range r.metas {
		if meta.Provider == provider {
			result = append(result, meta)
		}
	}

	return result
}

// Resolve は provider と name からサービスを取得します。
// 未登録の場合は models.ErrNotFound を返します。
func (r *Registry) Resolve(provider, name string) (Service, error) {
	key := fmt.Sprintf("%s:%s", provider, name)

	r.mu.RLock()
	defer r.mu.RUnlock()

	svc, exists := r.services[key]
	if !exists {
		return nil, fmt.Errorf("service %q: %w", key, models.ErrNotFound)
	}

	return svc, nil
}

// SharedBackend は共有バックエンドを登録または取得します。
// backend が 1 つ以上渡された場合は最初の値を name に紐付けて登録し、その値を返します。
// backend が渡されない場合は登録済みの値を返します（未登録の場合は nil）。
func (r *Registry) SharedBackend(name string, backend ...any) any {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(backend) > 0 {
		r.backends[name] = backend[0]
		return backend[0]
	}

	return r.backends[name]
}

// InitAll は登録順に全サービスの Init を呼び出します。
// 1 つでもエラーが返った場合はそこで中断してエラーを返します。
func (r *Registry) InitAll(ctx context.Context, deps ServiceDeps) error {
	r.mu.RLock()
	keys := make([]string, len(r.order))
	copy(keys, r.order)
	services := make(map[string]Service, len(r.services))
	for k, v := range r.services {
		services[k] = v
	}
	r.mu.RUnlock()

	for _, key := range keys {
		if err := services[key].Init(ctx, deps); err != nil {
			return fmt.Errorf("init service %q: %w", key, err)
		}
	}

	return nil
}

// ShutdownAll は登録の逆順に全サービスの Shutdown を呼び出します。
// エラーが発生しても全サービスを実行し、最初に発生したエラーを返します。
func (r *Registry) ShutdownAll(ctx context.Context) error {
	r.mu.RLock()
	keys := make([]string, len(r.order))
	copy(keys, r.order)
	services := make(map[string]Service, len(r.services))
	for k, v := range r.services {
		services[k] = v
	}
	r.mu.RUnlock()

	var firstErr error

	for i := len(keys) - 1; i >= 0; i-- {
		key := keys[i]
		if err := services[key].Shutdown(ctx); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("shutdown service %q: %w", key, err)
			}
		}
	}

	return firstErr
}

// HealthAll は全サービスのヘルスステータスを収集して返します。
// キーは "provider:name" 形式です。
func (r *Registry) HealthAll(ctx context.Context) map[string]HealthStatus {
	r.mu.RLock()
	keys := make([]string, len(r.order))
	copy(keys, r.order)
	services := make(map[string]Service, len(r.services))
	for k, v := range r.services {
		services[k] = v
	}
	r.mu.RUnlock()

	result := make(map[string]HealthStatus, len(keys))
	for _, key := range keys {
		result[key] = services[key].Health(ctx)
	}

	return result
}
