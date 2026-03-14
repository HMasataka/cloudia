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

// DiskUsageChecker can report current disk usage in bytes.
type DiskUsageChecker interface {
	DiskUsageBytes(ctx context.Context) (int64, error)
}

// Limiter はリソース制限を管理します。
type Limiter struct {
	store         state.Store
	maxContainers int
	defaultCPU    int64
	defaultMemory int64
	storageQuota  int64 // bytes; 0 means unlimited
	diskChecker   DiskUsageChecker
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

	var storageQuota int64
	if cfg.StorageQuota != "" {
		storageQuota, err = units.RAMInBytes(cfg.StorageQuota)
		if err != nil {
			return nil, fmt.Errorf("resource: failed to parse storage_quota %q: %w", cfg.StorageQuota, err)
		}
	}

	return &Limiter{
		store:         store,
		maxContainers: cfg.MaxContainers,
		defaultCPU:    nanoCPU,
		defaultMemory: memBytes,
		storageQuota:  storageQuota,
	}, nil
}

// SetDiskChecker sets the DiskUsageChecker used by CheckDiskUsage.
// Call this after creating the Limiter if disk usage checking is desired.
func (l *Limiter) SetDiskChecker(dc DiskUsageChecker) {
	l.diskChecker = dc
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

// CheckDiskUsage はDocker のディスク使用量が storage_quota を超えていないかチェックします。
// diskChecker が設定されていない場合、または storageQuota が 0 の場合は常に nil を返します。
// 使用量が quota を超えている場合は models.ErrLimitExceeded をラップしたエラーを返します。
func (l *Limiter) CheckDiskUsage(ctx context.Context) error {
	if l.diskChecker == nil || l.storageQuota <= 0 {
		return nil
	}
	used, err := l.diskChecker.DiskUsageBytes(ctx)
	if err != nil {
		return fmt.Errorf("resource: failed to check disk usage: %w", err)
	}
	if used >= l.storageQuota {
		return fmt.Errorf("disk quota exceeded: used %d bytes, quota %d bytes: %w", used, l.storageQuota, models.ErrLimitExceeded)
	}
	return nil
}
