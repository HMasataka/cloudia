package resource

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
)

func newTestLimiterConfig(maxContainers int) config.LimitsConfig {
	return config.LimitsConfig{
		MaxContainers: maxContainers,
		DefaultCPU:    "1",
		DefaultMemory: "512m",
	}
}

func newResourceInStore(store state.Store, kind, id, status string) {
	r := &models.Resource{
		Kind:      kind,
		ID:        id,
		Provider:  "aws",
		Service:   "ec2",
		Region:    "us-east-1",
		Status:    status,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	store.Put(context.Background(), r) //nolint
}

func TestLimiter_UnderLimit_ReturnsNil(t *testing.T) {
	store := state.NewMemoryStore()
	newResourceInStore(store, "k", "id1", "active")

	l, err := NewLimiter(store, newTestLimiterConfig(3))
	if err != nil {
		t.Fatalf("NewLimiter failed: %v", err)
	}

	if err := l.CheckContainerLimit(context.Background()); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestLimiter_AtLimit_ReturnsErrLimitExceeded(t *testing.T) {
	store := state.NewMemoryStore()
	newResourceInStore(store, "k", "id1", "active")
	newResourceInStore(store, "k", "id2", "active")
	newResourceInStore(store, "k", "id3", "active")

	l, err := NewLimiter(store, newTestLimiterConfig(3))
	if err != nil {
		t.Fatalf("NewLimiter failed: %v", err)
	}

	err = l.CheckContainerLimit(context.Background())
	if !errors.Is(err, models.ErrLimitExceeded) {
		t.Errorf("expected ErrLimitExceeded, got %v", err)
	}
}

func TestLimiter_TerminatedAndOrphanNotCounted(t *testing.T) {
	store := state.NewMemoryStore()
	// terminated と orphan はアクティブとして数えない
	newResourceInStore(store, "k", "id1", "terminated")
	newResourceInStore(store, "k", "id2", "orphan")
	newResourceInStore(store, "k", "id3", "active") // 1 active

	l, err := NewLimiter(store, newTestLimiterConfig(2))
	if err != nil {
		t.Fatalf("NewLimiter failed: %v", err)
	}

	if err := l.CheckContainerLimit(context.Background()); err != nil {
		t.Errorf("terminated/orphan should not count, expected nil, got %v", err)
	}
}

func TestLimiter_ErrorMessageContainsCurrentAndMax(t *testing.T) {
	store := state.NewMemoryStore()
	newResourceInStore(store, "k", "id1", "active")
	newResourceInStore(store, "k", "id2", "active")

	l, err := NewLimiter(store, newTestLimiterConfig(2))
	if err != nil {
		t.Fatalf("NewLimiter failed: %v", err)
	}

	err = l.CheckContainerLimit(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	// エラーメッセージに "current 2" と "max 2" が含まれること
	if !containsStr(msg, "current 2") {
		t.Errorf("error message should contain 'current 2', got: %q", msg)
	}
	if !containsStr(msg, "max 2") {
		t.Errorf("error message should contain 'max 2', got: %q", msg)
	}
}

func TestLimiter_DefaultResources_CPU(t *testing.T) {
	store := state.NewMemoryStore()
	l, err := NewLimiter(store, config.LimitsConfig{
		MaxContainers: 10,
		DefaultCPU:    "1",
		DefaultMemory: "512m",
	})
	if err != nil {
		t.Fatalf("NewLimiter failed: %v", err)
	}

	cpu, _ := l.DefaultResources()
	const want int64 = 1_000_000_000
	if cpu != want {
		t.Errorf("expected CPU=%d (1 NanoCPU), got %d", want, cpu)
	}
}

func TestLimiter_DefaultResources_Memory(t *testing.T) {
	store := state.NewMemoryStore()
	l, err := NewLimiter(store, config.LimitsConfig{
		MaxContainers: 10,
		DefaultCPU:    "1",
		DefaultMemory: "512m",
	})
	if err != nil {
		t.Fatalf("NewLimiter failed: %v", err)
	}

	_, mem := l.DefaultResources()
	const want int64 = 536870912 // 512 * 1024 * 1024
	if mem != want {
		t.Errorf("expected memory=%d (512MiB), got %d", want, mem)
	}
}

func TestLimiter_InvalidCPU_ReturnsError(t *testing.T) {
	store := state.NewMemoryStore()
	_, err := NewLimiter(store, config.LimitsConfig{
		MaxContainers: 10,
		DefaultCPU:    "not-a-number",
		DefaultMemory: "512m",
	})
	if err == nil {
		t.Fatal("expected error for invalid CPU, got nil")
	}
}

func TestLimiter_InvalidMemory_ReturnsError(t *testing.T) {
	store := state.NewMemoryStore()
	_, err := NewLimiter(store, config.LimitsConfig{
		MaxContainers: 10,
		DefaultCPU:    "1",
		DefaultMemory: "not-a-memory",
	})
	if err == nil {
		t.Fatal("expected error for invalid memory, got nil")
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstr(s, sub))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
