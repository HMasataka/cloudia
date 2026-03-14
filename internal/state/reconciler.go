package state

import (
	"context"
	"time"

	"github.com/HMasataka/cloudia/pkg/models"
	"github.com/docker/docker/api/types/container"
	"go.uber.org/zap"
)

// DockerLister はマネージドコンテナ一覧を取得するインターフェースです。
type DockerLister interface {
	ListManagedContainers(ctx context.Context) ([]container.Summary, error)
}

// Reconciler は State と Docker の差分を定期的に解消します。
type Reconciler struct {
	store    Store
	locker   *LockManager
	docker   DockerLister
	interval time.Duration
	logger   *zap.Logger
}

// NewReconciler は Reconciler を返します。
func NewReconciler(store Store, locker *LockManager, docker DockerLister, interval time.Duration, logger *zap.Logger) *Reconciler {
	return &Reconciler{
		store:    store,
		locker:   locker,
		docker:   docker,
		interval: interval,
		logger:   logger,
	}
}

// Start はバックグラウンド goroutine で定期照合を開始します。
// ctx がキャンセルされると goroutine は停止します。
func (r *Reconciler) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := r.Reconcile(ctx); err != nil {
					r.logger.Warn("reconcile failed", zap.Error(err))
				}
			}
		}
	}()
}

// Reconcile は1回の照合を実行します。
// - State にあるが Docker にない → Status を "terminated" に更新
// - Docker にあるが State にない → "orphan" ステータスで新規 Resource を追加
func (r *Reconciler) Reconcile(ctx context.Context) error {
	containers, err := r.docker.ListManagedContainers(ctx)
	if err != nil {
		r.logger.Warn("reconcile: failed to list docker containers, skipping", zap.Error(err))
		return nil
	}

	// Docker コンテナ ID → container.Summary のマップを構築
	dockerByID := make(map[string]container.Summary, len(containers))
	for _, c := range containers {
		dockerByID[c.ID] = c
	}

	// State の全リソースを取得
	stateResources, err := r.store.List(ctx, "", nil)
	if err != nil {
		return err
	}

	// State にあるが Docker にない → "terminated" に更新
	for _, res := range stateResources {
		if res.ContainerID == "" {
			continue
		}
		if _, exists := dockerByID[res.ContainerID]; exists {
			continue
		}
		if !r.locker.TryLock(res.Kind, res.ID) {
			r.logger.Warn("reconcile: skip terminated update, resource locked",
				zap.String("kind", res.Kind),
				zap.String("id", res.ID),
			)
			continue
		}
		res.Status = "terminated"
		res.UpdatedAt = time.Now()
		if putErr := r.store.Put(ctx, res); putErr != nil {
			r.logger.Warn("reconcile: failed to update terminated resource",
				zap.String("kind", res.Kind),
				zap.String("id", res.ID),
				zap.Error(putErr),
			)
		}
		r.locker.Unlock(res.Kind, res.ID)
	}

	// State に登録済みの ContainerID セットを構築
	knownContainerIDs := make(map[string]struct{}, len(stateResources))
	for _, res := range stateResources {
		if res.ContainerID != "" {
			knownContainerIDs[res.ContainerID] = struct{}{}
		}
	}

	// Docker にあるが State にない → "orphan" で新規 Resource を追加
	for _, c := range containers {
		if _, known := knownContainerIDs[c.ID]; known {
			continue
		}

		kind := c.Labels["cloudia.kind"]
		region := c.Labels["cloudia.region"]
		provider := c.Labels["cloudia.provider"]
		service := c.Labels["cloudia.service"]

		// Docker コンテナ ID 先頭 12 文字を Resource ID として使用
		id := c.ID
		if len(id) > 12 {
			id = id[:12]
		}

		if !r.locker.TryLock(kind, id) {
			r.logger.Warn("reconcile: skip orphan insert, resource locked",
				zap.String("kind", kind),
				zap.String("id", id),
			)
			continue
		}

		now := time.Now()
		orphan := &models.Resource{
			Kind:        kind,
			ID:          id,
			Provider:    provider,
			Service:     service,
			Region:      region,
			Status:      "orphan",
			ContainerID: c.ID,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if putErr := r.store.Put(ctx, orphan); putErr != nil {
			r.logger.Warn("reconcile: failed to insert orphan resource",
				zap.String("container_id", c.ID),
				zap.Error(putErr),
			)
		}
		r.locker.Unlock(kind, id)
	}

	return nil
}
