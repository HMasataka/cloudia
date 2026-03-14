package resource

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
)

// mockDockerRemover は DockerRemover のモック実装です。
type mockDockerRemover struct {
	stopErr   error
	removeErr error
	stopped   []string
	removed   []string
}

func (m *mockDockerRemover) StopContainer(_ context.Context, id string, _ *int) error {
	if m.stopErr != nil {
		return m.stopErr
	}
	m.stopped = append(m.stopped, id)
	return nil
}

func (m *mockDockerRemover) RemoveContainer(_ context.Context, id string) error {
	if m.removeErr != nil {
		return m.removeErr
	}
	m.removed = append(m.removed, id)
	return nil
}

func newTestResource(kind, id, status, containerID string) *models.Resource {
	return &models.Resource{
		Kind:        kind,
		ID:          id,
		Provider:    "aws",
		Service:     "ec2",
		Region:      "us-east-1",
		Status:      status,
		ContainerID: containerID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

func TestCleaner_CleanupOrphans_RemovesOrphanAndTerminated(t *testing.T) {
	store := state.NewMemoryStore()
	ctx := context.Background()

	store.Put(ctx, newTestResource("k", "orphan1", "orphan", "cid-orphan1"))
	store.Put(ctx, newTestResource("k", "term1", "terminated", "cid-term1"))
	store.Put(ctx, newTestResource("k", "active1", "active", "cid-active1"))

	docker := &mockDockerRemover{}
	cleaner := NewCleaner(store, docker, zap.NewNop())

	deleted, err := cleaner.CleanupOrphans(ctx)
	if err != nil {
		t.Fatalf("CleanupOrphans failed: %v", err)
	}
	if deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", deleted)
	}

	// orphan と terminated は State から削除されている
	_, err = store.Get(ctx, "k", "orphan1")
	if !errors.Is(err, models.ErrNotFound) {
		t.Errorf("orphan1 should be removed from state")
	}
	_, err = store.Get(ctx, "k", "term1")
	if !errors.Is(err, models.ErrNotFound) {
		t.Errorf("term1 should be removed from state")
	}

	// active は State に残っている
	_, err = store.Get(ctx, "k", "active1")
	if err != nil {
		t.Errorf("active1 should remain in state: %v", err)
	}
}

func TestCleaner_CleanupOrphans_ActiveNotAffected(t *testing.T) {
	store := state.NewMemoryStore()
	ctx := context.Background()

	store.Put(ctx, newTestResource("k", "active1", "active", ""))
	store.Put(ctx, newTestResource("k", "active2", "creating", ""))

	docker := &mockDockerRemover{}
	cleaner := NewCleaner(store, docker, zap.NewNop())

	deleted, err := cleaner.CleanupOrphans(ctx)
	if err != nil {
		t.Fatalf("CleanupOrphans failed: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", deleted)
	}
}

func TestCleaner_CleanupOrphans_DockerStopFailSkips(t *testing.T) {
	store := state.NewMemoryStore()
	ctx := context.Background()

	store.Put(ctx, newTestResource("k", "orphan1", "orphan", "cid-orphan1"))

	docker := &mockDockerRemover{stopErr: errors.New("stop failed")}
	cleaner := NewCleaner(store, docker, zap.NewNop())

	deleted, err := cleaner.CleanupOrphans(ctx)
	if err != nil {
		t.Fatalf("CleanupOrphans should not return error on Docker stop failure, got: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted (skipped), got %d", deleted)
	}

	// State からも削除されていない（スキップ）
	_, err = store.Get(ctx, "k", "orphan1")
	if err != nil {
		t.Errorf("orphan1 should remain in state after Docker stop failure")
	}
}

func TestCleaner_CleanupOrphans_DockerRemoveFailSkips(t *testing.T) {
	store := state.NewMemoryStore()
	ctx := context.Background()

	store.Put(ctx, newTestResource("k", "orphan2", "orphan", "cid-orphan2"))

	docker := &mockDockerRemover{removeErr: errors.New("remove failed")}
	cleaner := NewCleaner(store, docker, zap.NewNop())

	deleted, err := cleaner.CleanupOrphans(ctx)
	if err != nil {
		t.Fatalf("CleanupOrphans should not return error on Docker remove failure, got: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted (skipped), got %d", deleted)
	}

	// State からも削除されていない（スキップ）
	_, err = store.Get(ctx, "k", "orphan2")
	if err != nil {
		t.Errorf("orphan2 should remain in state after Docker remove failure")
	}
}

func TestCleaner_CleanupOrphans_NoContainerID(t *testing.T) {
	store := state.NewMemoryStore()
	ctx := context.Background()

	// ContainerID 空の orphan は Docker 操作なしに State から削除
	store.Put(ctx, newTestResource("k", "orphan3", "orphan", ""))

	docker := &mockDockerRemover{}
	cleaner := NewCleaner(store, docker, zap.NewNop())

	deleted, err := cleaner.CleanupOrphans(ctx)
	if err != nil {
		t.Fatalf("CleanupOrphans failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	if len(docker.stopped) != 0 {
		t.Errorf("expected no Docker stop calls, got %d", len(docker.stopped))
	}
}
