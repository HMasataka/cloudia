package resource

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/state"
)

// TTLManager は TTL 付きリソースの期限切れを定期的にチェックし、
// 期限切れリソースの Status を "terminated" に変更します。
type TTLManager struct {
	store    state.Store
	cleaner  *Cleaner
	interval time.Duration
	enabled  bool
	logger   *zap.Logger
}

// NewTTLManager は TTLManager を生成します。
// enabled が false の場合、Start を呼んでも goroutine を起動しません。
func NewTTLManager(store state.Store, cleaner *Cleaner, interval time.Duration, enabled bool, logger *zap.Logger) *TTLManager {
	return &TTLManager{
		store:    store,
		cleaner:  cleaner,
		interval: interval,
		enabled:  enabled,
		logger:   logger,
	}
}

// Start はバックグラウンド goroutine で TTL チェックを定期実行します。
// ctx がキャンセルされると停止します。enabled が false の場合は goroutine を起動しません。
func (m *TTLManager) Start(ctx context.Context) {
	if !m.enabled {
		return
	}
	go func() {
		ticker := time.NewTicker(m.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := m.checkTTL(ctx); err != nil {
					m.logger.Warn("ttl: check failed", zap.Error(err))
				}
				if _, err := m.cleaner.CleanupOrphans(ctx); err != nil {
					m.logger.Warn("ttl: cleanup failed", zap.Error(err))
				}
			}
		}
	}()
}

// checkTTL は全リソースを走査し、TTL 期限切れのリソースの Status を "terminated" に変更します。
func (m *TTLManager) checkTTL(ctx context.Context) error {
	resources, err := m.store.List(ctx, "", state.Filter{})
	if err != nil {
		return fmt.Errorf("ttl: failed to list resources: %w", err)
	}

	now := time.Now()
	for _, r := range resources {
		if r.TTL == nil {
			// TTL = nil は永続リソース
			continue
		}

		expiredAt := r.CreatedAt.Add(*r.TTL)
		if expiredAt.Before(now) {
			r.Status = "terminated"
			r.UpdatedAt = now
			if err := m.store.Put(ctx, r); err != nil {
				m.logger.Warn("ttl: failed to mark resource as terminated",
					zap.String("resource_id", r.ID),
					zap.String("kind", r.Kind),
					zap.Error(err),
				)
			}
		}
	}

	return nil
}
