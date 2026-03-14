package rdb_test

import (
	"context"
	"strings"
	"testing"

	"github.com/HMasataka/cloudia/internal/backend/docker"
	"github.com/HMasataka/cloudia/internal/backend/rdb"
	"github.com/HMasataka/cloudia/internal/service"
	"go.uber.org/zap"
)

// mockEngine は Engine インターフェースのモック実装です。
// HealthCheck は常に成功を返します（TCP 接続不要）。
type mockEngine struct {
	image       string
	defaultPort string
	envPrefix   string
}

func (e *mockEngine) Image() string { return e.image }

func (e *mockEngine) DefaultPort() string { return e.defaultPort }

func (e *mockEngine) ContainerName() string { return "cloudia-mock" }

func (e *mockEngine) Env(rootPassword string) []string {
	return []string{e.envPrefix + "_PASSWORD=" + rootPassword}
}

func (e *mockEngine) HealthCheck(host, port string) error {
	// テスト用: 常に成功
	return nil
}

// stubContainerRunner はコンテナ操作をスタブ化します。
type stubContainerRunner struct{}

func (s *stubContainerRunner) RunContainer(_ context.Context, _ docker.ContainerConfig) (string, error) {
	return "stub-container-id", nil
}

func (s *stubContainerRunner) StopContainer(_ context.Context, _ string, _ *int) error {
	return nil
}

func (s *stubContainerRunner) RemoveContainer(_ context.Context, _ string) error {
	return nil
}

func (s *stubContainerRunner) StartContainer(_ context.Context, _ string) error {
	return nil
}

func (s *stubContainerRunner) PauseContainer(_ context.Context, _ string) error {
	return nil
}

func (s *stubContainerRunner) UnpauseContainer(_ context.Context, _ string) error {
	return nil
}

func (s *stubContainerRunner) InspectContainer(_ context.Context, _ string) (docker.ContainerInfo, error) {
	return docker.ContainerInfo{State: "running", IPAddress: "172.17.0.2"}, nil
}

func (s *stubContainerRunner) ExecInContainer(_ context.Context, _ string, _ []string) ([]byte, error) {
	return nil, nil
}

// stubPortAllocator はポート割り当てをスタブ化します。
type stubPortAllocator struct {
	allocated int
}

func (p *stubPortAllocator) Allocate(preferred int, _ string) (int, error) {
	if preferred > 0 {
		p.allocated = preferred
	} else {
		p.allocated = 13306
	}
	return p.allocated, nil
}

func (p *stubPortAllocator) Release(_ int) {}

// newTestRDBBackend は Docker/MySQL 依存なしで RDBBackend を初期化します。
func newTestRDBBackend(t *testing.T, engine rdb.Engine) *rdb.RDBBackend {
	t.Helper()
	backend := rdb.NewRDBBackend(engine, zap.NewNop())
	portAlloc := &stubPortAllocator{}
	runner := &stubContainerRunner{}
	deps := service.ServiceDeps{
		DockerClient:  runner,
		PortAllocator: portAlloc,
	}
	if err := backend.Init(context.Background(), deps); err != nil {
		t.Fatalf("RDBBackend.Init: %v", err)
	}
	return backend
}

// TestRDBBackend_MySQLEngine_Image は MySQLEngine.Image() が正しいイメージ名を返すことを検証します。
func TestRDBBackend_MySQLEngine_Image(t *testing.T) {
	engine := &rdb.MySQLEngine{}
	if got := engine.Image(); got != "mysql:8.0" {
		t.Errorf("MySQLEngine.Image() = %q, want %q", got, "mysql:8.0")
	}
}

// TestRDBBackend_MySQLEngine_DefaultPort は MySQLEngine.DefaultPort() が "3306" を返すことを検証します。
func TestRDBBackend_MySQLEngine_DefaultPort(t *testing.T) {
	engine := &rdb.MySQLEngine{}
	if got := engine.DefaultPort(); got != "3306" {
		t.Errorf("MySQLEngine.DefaultPort() = %q, want %q", got, "3306")
	}
}

// TestRDBBackend_MySQLEngine_Env は MySQLEngine.Env() が MYSQL_ROOT_PASSWORD を含むことを検証します。
func TestRDBBackend_MySQLEngine_Env(t *testing.T) {
	engine := &rdb.MySQLEngine{}
	env := engine.Env("testpassword")
	if len(env) == 0 {
		t.Fatal("MySQLEngine.Env() returned empty slice")
	}
	found := false
	for _, e := range env {
		if strings.Contains(e, "MYSQL_ROOT_PASSWORD=testpassword") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("MySQLEngine.Env() does not contain MYSQL_ROOT_PASSWORD=testpassword: %v", env)
	}
}

// TestRDBBackend_Init_Host は Init 後に Host() が "localhost" を返すことを検証します。
func TestRDBBackend_Init_Host(t *testing.T) {
	engine := &mockEngine{image: "mock:latest", defaultPort: "5432", envPrefix: "MOCK"}
	backend := newTestRDBBackend(t, engine)
	if got := backend.Host(); got != "localhost" {
		t.Errorf("Host() = %q, want %q", got, "localhost")
	}
}

// TestRDBBackend_Init_Port は Init 後に Port() が engine の DefaultPort に対応することを検証します。
func TestRDBBackend_Init_Port(t *testing.T) {
	engine := &mockEngine{image: "mock:latest", defaultPort: "5432", envPrefix: "MOCK"}
	backend := newTestRDBBackend(t, engine)
	port := backend.Port()
	if port == "" {
		t.Error("Port() returned empty string after Init")
	}
}

// TestRDBBackend_Init_RootPassword は Init 後に RootPassword() がデフォルトパスワードを返すことを検証します。
func TestRDBBackend_Init_RootPassword(t *testing.T) {
	engine := &mockEngine{image: "mock:latest", defaultPort: "3306", envPrefix: "MOCK"}
	backend := newTestRDBBackend(t, engine)
	if got := backend.RootPassword(); got != "cloudia" {
		t.Errorf("RootPassword() = %q, want %q", got, "cloudia")
	}
}

// TestRDBBackend_MockEngine_Interface は mockEngine が Engine インターフェースを満たすことを検証します。
func TestRDBBackend_MockEngine_Interface(t *testing.T) {
	var engine rdb.Engine = &mockEngine{
		image:       "postgres:16",
		defaultPort: "5432",
		envPrefix:   "POSTGRES",
	}

	if got := engine.Image(); got != "postgres:16" {
		t.Errorf("Engine.Image() = %q, want %q", got, "postgres:16")
	}

	if got := engine.DefaultPort(); got != "5432" {
		t.Errorf("Engine.DefaultPort() = %q, want %q", got, "5432")
	}

	env := engine.Env("secret")
	if len(env) == 0 {
		t.Fatal("Engine.Env() returned empty slice")
	}
	if !strings.Contains(env[0], "secret") {
		t.Errorf("Engine.Env() does not contain password: %v", env)
	}

	if err := engine.HealthCheck("localhost", "5432"); err != nil {
		t.Errorf("Engine.HealthCheck() returned unexpected error: %v", err)
	}
}

// TestRDBBackend_Shutdown は Shutdown が正常に完了することを検証します。
func TestRDBBackend_Shutdown(t *testing.T) {
	engine := &mockEngine{image: "mock:latest", defaultPort: "3306", envPrefix: "MOCK"}
	backend := newTestRDBBackend(t, engine)

	if err := backend.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown() returned unexpected error: %v", err)
	}
}
