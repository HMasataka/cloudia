package resource

import (
	"context"
	"fmt"
	"strconv"

	"github.com/docker/go-units"

	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
)

// Limiter はリソース制限を管理します。
type Limiter struct {
	store         state.Store
	maxContainers int
	defaultCPU    int64
	defaultMemory int64
}

// NewLimiter は Limiter を生成します。
// cfg.DefaultCPU は NanoCPUs（1CPU = 1_000_000_000）に変換し、
// cfg.DefaultMemory は docker/go-units の RAMInBytes でバイト数に変換します。
func NewLimiter(store state.Store, cfg config.LimitsConfig) (*Limiter, error) {
	cpuFloat, err := strconv.ParseFloat(cfg.DefaultCPU, 64)
	if err != nil {
		return nil, fmt.Errorf("resource: failed to parse default_cpu %q: %w", cfg.DefaultCPU, err)
	}
	nanoCPU := int64(cpuFloat * 1e9)

	memBytes, err := units.RAMInBytes(cfg.DefaultMemory)
	if err != nil {
		return nil, fmt.Errorf("resource: failed to parse default_memory %q: %w", cfg.DefaultMemory, err)
	}

	return &Limiter{
		store:         store,
		maxContainers: cfg.MaxContainers,
		defaultCPU:    nanoCPU,
		defaultMemory: memBytes,
	}, nil
}

// CheckContainerLimit は現在のアクティブリソース数が上限に達していないかを確認します。
// "terminated" または "orphan" 以外のリソースをアクティブとみなします。
// 上限に達している場合は models.ErrLimitExceeded をラップしたエラーを返します。
func (l *Limiter) CheckContainerLimit(ctx context.Context) error {
	all, err := l.store.List(ctx, "", state.Filter{})
	if err != nil {
		return fmt.Errorf("resource: failed to list resources: %w", err)
	}

	current := 0
	for _, r := range all {
		if r.Status != "terminated" && r.Status != "orphan" {
			current++
		}
	}

	if current >= l.maxContainers {
		return fmt.Errorf("container limit reached: current %d, max %d: %w", current, l.maxContainers, models.ErrLimitExceeded)
	}

	return nil
}

// DefaultResources はデフォルトの CPU（NanoCPUs）とメモリ（バイト）を返します。
func (l *Limiter) DefaultResources() (cpu int64, memory int64) {
	return l.defaultCPU, l.defaultMemory
}
