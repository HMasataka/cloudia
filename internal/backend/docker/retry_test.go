package docker

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestCalcBackoff_FirstAttempt(t *testing.T) {
	cfg := retryConfig{
		maxAttempts: 3,
		initialWait: 1 * time.Second,
		multiplier:  2,
		maxWait:     8 * time.Second,
		jitter:      0, // no jitter for deterministic test
	}
	got := calcBackoff(cfg, 0)
	// With jitter=0, first attempt (attempt=0) should equal initialWait.
	if got != cfg.initialWait {
		t.Errorf("calcBackoff(attempt=0) = %v, want %v", got, cfg.initialWait)
	}
}

func TestCalcBackoff_Doubles(t *testing.T) {
	cfg := retryConfig{
		maxAttempts: 3,
		initialWait: 1 * time.Second,
		multiplier:  2,
		maxWait:     8 * time.Second,
		jitter:      0,
	}
	a0 := calcBackoff(cfg, 0) // 1s
	a1 := calcBackoff(cfg, 1) // 2s
	a2 := calcBackoff(cfg, 2) // 4s
	if a0 != 1*time.Second {
		t.Errorf("attempt 0: got %v, want 1s", a0)
	}
	if a1 != 2*time.Second {
		t.Errorf("attempt 1: got %v, want 2s", a1)
	}
	if a2 != 4*time.Second {
		t.Errorf("attempt 2: got %v, want 4s", a2)
	}
}

func TestCalcBackoff_CapsAtMaxWait(t *testing.T) {
	cfg := retryConfig{
		maxAttempts: 10,
		initialWait: 1 * time.Second,
		multiplier:  2,
		maxWait:     8 * time.Second,
		jitter:      0,
	}
	// attempt 3 would be 8s (1*2*2*2), should be capped at maxWait
	got := calcBackoff(cfg, 3)
	if got > cfg.maxWait {
		t.Errorf("calcBackoff(attempt=3) = %v, exceeds maxWait %v", got, cfg.maxWait)
	}
}

func TestCalcBackoff_WithJitter_InRange(t *testing.T) {
	cfg := retryConfig{
		maxAttempts: 3,
		initialWait: 1 * time.Second,
		multiplier:  2,
		maxWait:     8 * time.Second,
		jitter:      0.25,
	}
	// Run many times and check jitter bounds
	for i := 0; i < 100; i++ {
		got := calcBackoff(cfg, 0)
		low := time.Duration(float64(cfg.initialWait) * (1 - cfg.jitter))
		high := time.Duration(float64(cfg.initialWait) * (1 + cfg.jitter))
		if got < low || got > high {
			t.Errorf("calcBackoff with jitter out of range: got %v, want [%v, %v]", got, low, high)
		}
	}
}

func TestIsPortConflictError_True(t *testing.T) {
	cases := []string{
		"port is already allocated",
		"address already in use",
		"bind: address already in use",
	}
	for _, msg := range cases {
		err := fmt.Errorf("%s", msg)
		if !isPortConflictError(err) {
			t.Errorf("expected isPortConflictError(%q) = true", msg)
		}
	}
}

func TestIsPortConflictError_False(t *testing.T) {
	cases := []string{
		"container not found",
		"image pull failed",
		"",
	}
	for _, msg := range cases {
		if msg == "" {
			if isPortConflictError(nil) {
				t.Error("expected isPortConflictError(nil) = false")
			}
			continue
		}
		err := fmt.Errorf("%s", msg)
		if isPortConflictError(err) {
			t.Errorf("expected isPortConflictError(%q) = false", msg)
		}
	}
}

func TestIsPortConflictError_BindFailed(t *testing.T) {
	err := fmt.Errorf("driver failed programming external connectivity on endpoint myapp: Bind for 0.0.0.0:8080 failed: port is already allocated")
	if !isPortConflictError(err) {
		t.Errorf("expected port conflict detection for bind failure message")
	}
}

// Verify that isPortConflictError uses strings package correctly.
func init() {
	_ = strings.Contains // ensure strings import is used
}
