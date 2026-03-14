package resource

import (
	"errors"
	"testing"

	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/pkg/models"
)

func newTestPortManager() *PortManager {
	return NewPortManager(config.PortConfig{
		RangeStart:     10000,
		RangeEnd:       10004, // 5 ports for easy exhaustion tests
		MaxPerResource: 3,
	})
}

func TestAllocate_PreferredAvailable(t *testing.T) {
	pm := newTestPortManager()
	port, err := pm.Allocate(10000, "res-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if port != 10000 {
		t.Errorf("expected 10000, got %d", port)
	}
}

func TestAllocate_PreferredInUse_Fallback(t *testing.T) {
	pm := newTestPortManager()
	// 10000 を先に割り当て
	if _, err := pm.Allocate(10000, "res-1"); err != nil {
		t.Fatal(err)
	}
	// 再度 10000 を指定すると 10001 が返るはず
	port, err := pm.Allocate(10000, "res-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if port != 10001 {
		t.Errorf("expected 10001, got %d", port)
	}
}

func TestAllocateAny_Success(t *testing.T) {
	pm := newTestPortManager()
	port, err := pm.AllocateAny("res-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if port < 10000 || port > 10004 {
		t.Errorf("port %d out of range", port)
	}
}

func TestAllocate_Exhausted(t *testing.T) {
	pm := newTestPortManager()
	// 全ポートを埋める（5ポート、MaxPerResource=3 なので複数リソースに割り当て）
	ids := []string{"r1", "r1", "r1", "r2", "r2"}
	for _, id := range ids {
		if _, err := pm.AllocateAny(id); err != nil {
			t.Fatalf("unexpected error while filling: %v", err)
		}
	}
	// 次の割り当ては失敗するはず
	_, err := pm.AllocateAny("r3")
	if !errors.Is(err, models.ErrLimitExceeded) {
		t.Errorf("expected ErrLimitExceeded, got %v", err)
	}
}

func TestAllocate_ExhaustedViaAllocate(t *testing.T) {
	pm := newTestPortManager()
	ids := []string{"r1", "r1", "r1", "r2", "r2"}
	for _, id := range ids {
		if _, err := pm.AllocateAny(id); err != nil {
			t.Fatalf("unexpected error while filling: %v", err)
		}
	}
	_, err := pm.Allocate(10000, "r3")
	if !errors.Is(err, models.ErrLimitExceeded) {
		t.Errorf("expected ErrLimitExceeded, got %v", err)
	}
}

func TestRelease_PortReusable(t *testing.T) {
	pm := newTestPortManager()
	port, err := pm.Allocate(10000, "res-1")
	if err != nil {
		t.Fatal(err)
	}
	pm.Release(port)
	// 解放後に再取得できる
	port2, err := pm.Allocate(10000, "res-2")
	if err != nil {
		t.Fatalf("unexpected error after release: %v", err)
	}
	if port2 != port {
		t.Errorf("expected reuse of port %d, got %d", port, port2)
	}
}

func TestIsAvailable(t *testing.T) {
	pm := newTestPortManager()
	if !pm.IsAvailable(10000) {
		t.Error("10000 should be available initially")
	}
	pm.Allocate(10000, "res-1") //nolint
	if pm.IsAvailable(10000) {
		t.Error("10000 should not be available after allocation")
	}
}

func TestMaxPerResource(t *testing.T) {
	pm := newTestPortManager() // MaxPerResource=3
	for i := 0; i < 3; i++ {
		if _, err := pm.AllocateAny("res-1"); err != nil {
			t.Fatalf("allocation %d failed: %v", i, err)
		}
	}
	_, err := pm.AllocateAny("res-1")
	if !errors.Is(err, models.ErrLimitExceeded) {
		t.Errorf("expected ErrLimitExceeded on 4th allocation, got %v", err)
	}
}

func TestRelease_UnknownPort(t *testing.T) {
	pm := newTestPortManager()
	// 割り当てていないポートの Release はパニックしない
	pm.Release(10000)
}
