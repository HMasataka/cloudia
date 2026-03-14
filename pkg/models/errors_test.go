package models_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/HMasataka/cloudia/pkg/models"
)

func TestSentinelErrors_Is(t *testing.T) {
	tests := []struct {
		name    string
		sentinel error
	}{
		{"ErrNotFound", models.ErrNotFound},
		{"ErrAlreadyExists", models.ErrAlreadyExists},
		{"ErrLimitExceeded", models.ErrLimitExceeded},
		{"ErrServiceUnavailable", models.ErrServiceUnavailable},
		{"ErrUnsupportedOperation", models.ErrUnsupportedOperation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// センチネルエラーそのものが errors.Is で判別できることを確認
			if !errors.Is(tt.sentinel, tt.sentinel) {
				t.Errorf("errors.Is(%v, %v) = false, want true", tt.sentinel, tt.sentinel)
			}

			// ラップされたエラーでも errors.Is で判別できることを確認
			wrapped := fmt.Errorf("wrapped: %w", tt.sentinel)
			if !errors.Is(wrapped, tt.sentinel) {
				t.Errorf("errors.Is(wrapped %v, %v) = false, want true", wrapped, tt.sentinel)
			}

			// 異なるセンチネルエラーとは一致しないことを確認
			other := errors.New("other error")
			if errors.Is(tt.sentinel, other) {
				t.Errorf("errors.Is(%v, other) = true, want false", tt.sentinel)
			}
		})
	}
}
