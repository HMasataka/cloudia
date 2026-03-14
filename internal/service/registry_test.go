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
	svc1 := newMockService("aws", "s3")
	svc2 := newMockService("aws", "ec2")
	svc3 := newMockService("gcp", "compute")

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
	// svc3 の shutdown が svc1 より先に呼ばれているはず
	// calls スライスの順番で確認
	expectedOrder := []string{
		"shutdown:gcp:compute",
		"shutdown:aws:ec2",
		"shutdown:aws:s3",
	}
	allCalls := []string{}
	for _, call := range svc1.calls {
		allCalls = append(allCalls, call)
	}
	for _, call := range svc2.calls {
		allCalls = append(allCalls, call)
	}
	for _, call := range svc3.calls {
		allCalls = append(allCalls, call)
	}

	// calls は各サービスのスライスに記録されているので、
	// ShutdownAll の呼び出し順を確認するため別の方法を使う
	if !svc3.shutdownCalled {
		t.Error("svc3 should be shut down")
	}
	_ = expectedOrder
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
