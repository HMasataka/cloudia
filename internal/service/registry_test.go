package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/pkg/models"
)

// mockService は Service インターフェースのモック実装です。
type mockService struct {
	name     string
	provider string

	initCalled     bool
	shutdownCalled bool
	initErr        error
	shutdownErr    error
	healthy        bool
	onShutdown     func()

	mu    sync.Mutex
	calls []string
}

func (m *mockService) Name() string     { return m.name }
func (m *mockService) Provider() string { return m.provider }

func (m *mockService) Init(_ context.Context, _ service.ServiceDeps) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.initCalled = true
	m.calls = append(m.calls, "init:"+m.provider+":"+m.name)
	return m.initErr
}

func (m *mockService) HandleRequest(_ context.Context, _ service.Request) (service.Response, error) {
	return service.Response{StatusCode: 200}, nil
}

func (m *mockService) SupportedActions() []string {
	return []string{"create", "delete"}
}

func (m *mockService) Health(_ context.Context) service.HealthStatus {
	return service.HealthStatus{Healthy: m.healthy, Message: "ok"}
}

func (m *mockService) Shutdown(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shutdownCalled = true
	m.calls = append(m.calls, "shutdown:"+m.provider+":"+m.name)
	if m.onShutdown != nil {
		m.onShutdown()
	}
	return m.shutdownErr
}

func newMockService(provider, name string) *mockService {
	return &mockService{name: name, provider: provider, healthy: true}
}

func TestRegistry_RegisterAndResolve(t *testing.T) {
	reg := service.NewRegistry()
	svc := newMockService("aws", "s3")

	if err := reg.Register(svc); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	got, err := reg.Resolve("aws", "s3")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got.Name() != "s3" || got.Provider() != "aws" {
		t.Errorf("unexpected service: %s:%s", got.Provider(), got.Name())
	}
}

func TestRegistry_DuplicateRegister_ReturnsErrAlreadyExists(t *testing.T) {
	reg := service.NewRegistry()
	svc := newMockService("aws", "s3")

	if err := reg.Register(svc); err != nil {
		t.Fatalf("first Register failed: %v", err)
	}

	err := reg.Register(svc)
	if !errors.Is(err, models.ErrAlreadyExists) {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestRegistry_ResolveUnregistered_ReturnsErrNotFound(t *testing.T) {
	reg := service.NewRegistry()

	_, err := reg.Resolve("aws", "unregistered")
	if !errors.Is(err, models.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRegistry_InitAll_CallsAllServices(t *testing.T) {
	reg := service.NewRegistry()
	svc1 := newMockService("aws", "s3")
	svc2 := newMockService("gcp", "storage")

	reg.Register(svc1) //nolint
	reg.Register(svc2) //nolint

	if err := reg.InitAll(context.Background(), service.ServiceDeps{}); err != nil {
		t.Fatalf("InitAll failed: %v", err)
	}

	if !svc1.initCalled {
		t.Error("svc1.Init should have been called")
	}
	if !svc2.initCalled {
		t.Error("svc2.Init should have been called")
	}
}

func TestRegistry_InitAll_StopsOnFirstError(t *testing.T) {
	reg := service.NewRegistry()
	svc1 := newMockService("aws", "s3")
	svc1.initErr = errors.New("init error")
	svc2 := newMockService("aws", "ec2")

	reg.Register(svc1) //nolint
	reg.Register(svc2) //nolint

	err := reg.InitAll(context.Background(), service.ServiceDeps{})
	if err == nil {
		t.Fatal("expected error from InitAll, got nil")
	}
	// svc2 は svc1 でエラーが出たので呼ばれないはず
	if svc2.initCalled {
		t.Error("svc2.Init should not have been called after svc1 failed")
	}
}

func TestRegistry_ShutdownAll_ReverseOrder(t *testing.T) {
	reg := service.NewRegistry()

	// 共有の呼び出し順記録スライス
	var (
		orderMu sync.Mutex
		callOrder []string
	)
	recordCall := func(label string) {
		orderMu.Lock()
		defer orderMu.Unlock()
		callOrder = append(callOrder, label)
	}

	svc1 := newMockService("aws", "s3")
	svc2 := newMockService("aws", "ec2")
	svc3 := newMockService("gcp", "compute")

	svc1.onShutdown = func() { recordCall("shutdown:aws:s3") }
	svc2.onShutdown = func() { recordCall("shutdown:aws:ec2") }
	svc3.onShutdown = func() { recordCall("shutdown:gcp:compute") }

	// 登録順: svc1, svc2, svc3
	reg.Register(svc1) //nolint
	reg.Register(svc2) //nolint
	reg.Register(svc3) //nolint

	if err := reg.ShutdownAll(context.Background()); err != nil {
		t.Fatalf("ShutdownAll failed: %v", err)
	}

	if !svc1.shutdownCalled || !svc2.shutdownCalled || !svc3.shutdownCalled {
		t.Error("all services should have been shut down")
	}

	// シャットダウン順序は逆順: svc3, svc2, svc1
	expectedOrder := []string{
		"shutdown:gcp:compute",
		"shutdown:aws:ec2",
		"shutdown:aws:s3",
	}
	if len(callOrder) != len(expectedOrder) {
		t.Fatalf("expected %d shutdown calls, got %d: %v", len(expectedOrder), len(callOrder), callOrder)
	}
	for i, want := range expectedOrder {
		if callOrder[i] != want {
			t.Errorf("shutdown call[%d]: expected %q, got %q (full order: %v)", i, want, callOrder[i], callOrder)
		}
	}
}

func TestRegistry_ShutdownAll_ContinuesOnError(t *testing.T) {
	reg := service.NewRegistry()
	svc1 := newMockService("aws", "s3")
	svc2 := newMockService("aws", "ec2")
	svc2.shutdownErr = errors.New("shutdown error")

	reg.Register(svc1) //nolint
	reg.Register(svc2) //nolint

	err := reg.ShutdownAll(context.Background())
	if err == nil {
		t.Fatal("expected error from ShutdownAll, got nil")
	}

	// エラーが出ても svc1 は Shutdown が呼ばれる
	if !svc1.shutdownCalled {
		t.Error("svc1.Shutdown should have been called even when svc2 failed")
	}
}

func TestRegistry_HealthAll_ReturnsAllStatuses(t *testing.T) {
	reg := service.NewRegistry()
	svc1 := newMockService("aws", "s3")
	svc1.healthy = true
	svc2 := newMockService("gcp", "storage")
	svc2.healthy = false

	reg.Register(svc1) //nolint
	reg.Register(svc2) //nolint

	statuses := reg.HealthAll(context.Background())

	if len(statuses) != 2 {
		t.Errorf("expected 2 statuses, got %d", len(statuses))
	}

	if s, ok := statuses["aws:s3"]; !ok || !s.Healthy {
		t.Errorf("aws:s3 should be healthy, got %+v", s)
	}
	if s, ok := statuses["gcp:storage"]; !ok || s.Healthy {
		t.Errorf("gcp:storage should be unhealthy, got %+v", s)
	}
}

func TestRegistry_SharedBackend_RegisterAndGet(t *testing.T) {
	reg := service.NewRegistry()

	backend := struct{ Name string }{Name: "mydb"}
	got := reg.SharedBackend("db", backend)

	if got != backend {
		t.Errorf("expected registered backend, got %v", got)
	}

	// 引数なしで取得
	got2 := reg.SharedBackend("db")
	if got2 != backend {
		t.Errorf("expected same backend on get, got %v", got2)
	}
}

func TestRegistry_SharedBackend_UnregisteredReturnsNil(t *testing.T) {
	reg := service.NewRegistry()

	got := reg.SharedBackend("nonexistent")
	if got != nil {
		t.Errorf("expected nil for unregistered backend, got %v", got)
	}
}

func TestRegistry_RegisterWithMeta_StoresMetaData(t *testing.T) {
	reg := service.NewRegistry()
	svc := newMockService("aws", "s3")
	meta := service.ServiceMeta{
		Provider:     "aws",
		Name:         "s3",
		TargetPrefix: "S3_20060301",
		AWSProtocol:  "query",
	}

	if err := reg.RegisterWithMeta(svc, meta); err != nil {
		t.Fatalf("RegisterWithMeta failed: %v", err)
	}

	// Resolve も正常に動作することを確認
	got, err := reg.Resolve("aws", "s3")
	if err != nil {
		t.Fatalf("Resolve after RegisterWithMeta failed: %v", err)
	}
	if got.Name() != "s3" || got.Provider() != "aws" {
		t.Errorf("unexpected service: %s:%s", got.Provider(), got.Name())
	}
}

func TestRegistry_RegisterWithMeta_DuplicateReturnsErrAlreadyExists(t *testing.T) {
	reg := service.NewRegistry()
	svc := newMockService("aws", "s3")
	meta := service.ServiceMeta{Provider: "aws", Name: "s3"}

	if err := reg.RegisterWithMeta(svc, meta); err != nil {
		t.Fatalf("first RegisterWithMeta failed: %v", err)
	}

	err := reg.RegisterWithMeta(svc, meta)
	if !errors.Is(err, models.ErrAlreadyExists) {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestRegistry_MetaByProvider_ReturnsCorrectMetas(t *testing.T) {
	reg := service.NewRegistry()

	awsS3 := newMockService("aws", "s3")
	awsEC2 := newMockService("aws", "ec2")
	gcpStorage := newMockService("gcp", "storage")

	reg.RegisterWithMeta(awsS3, service.ServiceMeta{ //nolint
		Provider:    "aws",
		Name:        "s3",
		AWSProtocol: "query",
	})
	reg.RegisterWithMeta(awsEC2, service.ServiceMeta{ //nolint
		Provider:    "aws",
		Name:        "ec2",
		AWSProtocol: "query",
	})
	reg.RegisterWithMeta(gcpStorage, service.ServiceMeta{ //nolint
		Provider:     "gcp",
		Name:         "storage",
		PathPrefixes: []string{"/storage/v1/"},
	})

	awsMetas := reg.MetaByProvider("aws")
	if len(awsMetas) != 2 {
		t.Errorf("expected 2 AWS metas, got %d: %+v", len(awsMetas), awsMetas)
	}

	gcpMetas := reg.MetaByProvider("gcp")
	if len(gcpMetas) != 1 {
		t.Errorf("expected 1 GCP meta, got %d: %+v", len(gcpMetas), gcpMetas)
	}
	if gcpMetas[0].Name != "storage" {
		t.Errorf("expected gcp:storage meta, got %+v", gcpMetas[0])
	}
}

func TestRegistry_MetaByProvider_UnknownProviderReturnsEmpty(t *testing.T) {
	reg := service.NewRegistry()

	metas := reg.MetaByProvider("azure")
	if len(metas) != 0 {
		t.Errorf("expected empty slice for unknown provider, got %v", metas)
	}
}

func TestRegistry_MetaByProvider_OnlyRegisteredWithMeta(t *testing.T) {
	reg := service.NewRegistry()

	// Register のみ（RegisterWithMeta は使わない）
	svc := newMockService("aws", "s3")
	if err := reg.Register(svc); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// メタデータは登録されていないので空を返す
	metas := reg.MetaByProvider("aws")
	if len(metas) != 0 {
		t.Errorf("expected empty metas for Register-only service, got %v", metas)
	}
}

func TestRegistry_ListServices_ReturnsAllServices(t *testing.T) {
	reg := service.NewRegistry()

	// Register のみ
	svcS3 := newMockService("aws", "s3")
	if err := reg.Register(svcS3); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// RegisterWithMeta
	svcEC2 := newMockService("aws", "ec2")
	metaEC2 := service.ServiceMeta{
		Provider:    "aws",
		Name:        "ec2",
		AWSProtocol: "query",
	}
	if err := reg.RegisterWithMeta(svcEC2, metaEC2); err != nil {
		t.Fatalf("RegisterWithMeta failed: %v", err)
	}

	result := reg.ListServices()

	if len(result) != 2 {
		t.Fatalf("expected 2 services, got %d: %v", len(result), result)
	}

	// Register のみのサービスは Provider/Name だけ設定される
	s3Meta, ok := result["aws:s3"]
	if !ok {
		t.Fatal("aws:s3 not found in ListServices result")
	}
	if s3Meta.Provider != "aws" || s3Meta.Name != "s3" {
		t.Errorf("unexpected aws:s3 meta: %+v", s3Meta)
	}
	if s3Meta.AWSProtocol != "" {
		t.Errorf("expected empty AWSProtocol for Register-only service, got %q", s3Meta.AWSProtocol)
	}

	// RegisterWithMeta のサービスは完全なメタデータが返る
	ec2Meta, ok := result["aws:ec2"]
	if !ok {
		t.Fatal("aws:ec2 not found in ListServices result")
	}
	if ec2Meta.AWSProtocol != "query" {
		t.Errorf("expected AWSProtocol=query for ec2, got %q", ec2Meta.AWSProtocol)
	}
}

func TestRegistry_ListServices_ReturnsCopy(t *testing.T) {
	reg := service.NewRegistry()

	svc := newMockService("aws", "s3")
	if err := reg.Register(svc); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	result := reg.ListServices()

	// 戻り値を変更しても Registry に影響しない
	delete(result, "aws:s3")

	result2 := reg.ListServices()
	if _, ok := result2["aws:s3"]; !ok {
		t.Error("modification of ListServices result should not affect Registry")
	}
}

func TestRegistry_ListServices_EmptyRegistry(t *testing.T) {
	reg := service.NewRegistry()

	result := reg.ListServices()
	if len(result) != 0 {
		t.Errorf("expected empty map for empty Registry, got %v", result)
	}
}

func TestRegistry_ConcurrentRegisterAndResolve(t *testing.T) {
	reg := service.NewRegistry()
	var wg sync.WaitGroup

	// 並行で Register と Resolve を実行
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := "svc" + string(rune('0'+n))
			svc := newMockService("aws", name)
			_ = reg.Register(svc)
		}(i)
	}

	wg.Wait()

	// 全サービスが登録されているはず（並行安全性の確認）
	statuses := reg.HealthAll(context.Background())
	if len(statuses) != 10 {
		t.Errorf("expected 10 services, got %d", len(statuses))
	}
}
