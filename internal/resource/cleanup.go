package resource

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/state"
)

// DockerRemover は Docker コンテナの停止・削除を行うインターフェースです。
// docker.Client はこのインターフェースを満たすアダプタ経由で使用します。
type DockerRemover interface {
	StopContainer(ctx context.Context, id string, timeout *int) error
	RemoveContainer(ctx context.Context, id string) error
}

// Cleaner はオーファン・終了済みリソースのクリーンアップを担当します。
type Cleaner struct {
	store  state.Store
	docker DockerRemover
	logger *zap.Logger
}

// NewCleaner は Cleaner を生成します。
func NewCleaner(store state.Store, docker DockerRemover, logger *zap.Logger) *Cleaner {
	return &Cleaner{
		store:  store,
		docker: docker,
		logger: logger,
	}
}

// CleanupOrphans は Status が "orphan" または "terminated" のリソースを削除します。
// ContainerID が空でない場合は Docker コンテナを停止・削除します。
// Docker 削除に失敗した場合はログ警告してスキップ（State からも削除しない）。
// 削除に成功したリソース数を返します。
func (c *Cleaner) CleanupOrphans(ctx context.Context) (int, error) {
	resources, err := c.store.List(ctx, "", state.Filter{})
	if err != nil {
		return 0, fmt.Errorf("cleanup: failed to list resources: %w", err)
	}

	deleted := 0
	for _, r := range resources {
		if r.Status != "orphan" && r.Status != "terminated" {
			continue
		}

		if r.ContainerID != "" {
			if err := c.docker.StopContainer(ctx, r.ContainerID, nil); err != nil {
				c.logger.Warn("cleanup: failed to stop container, skipping",
					zap.String("resource_id", r.ID),
					zap.String("container_id", r.ContainerID),
					zap.Error(err),
				)
				continue
			}
			if err := c.docker.RemoveContainer(ctx, r.ContainerID); err != nil {
				c.logger.Warn("cleanup: failed to remove container, skipping",
					zap.String("resource_id", r.ID),
					zap.String("container_id", r.ContainerID),
					zap.Error(err),
				)
				continue
			}
		}

		if err := c.store.Delete(ctx, r.Kind, r.ID); err != nil {
			return deleted, fmt.Errorf("cleanup: failed to delete resource %s/%s: %w", r.Kind, r.ID, err)
		}
		deleted++
	}

	return deleted, nil
}
