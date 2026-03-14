package state_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/state"
)

// mockDockerLister は DockerLister のモック実装です。
type mockDockerLister struct {
	containers []container.Summary
	err        error
}

func (m *mockDockerLister) ListManagedContainers(_ context.Context) ([]container.Summary, error) {
	return m.containers, m.err
}

// mockContainerRemover は ContainerRemover のモック実装です。
type mockContainerRemover struct {
	removed []string
	err     error
}

func (m *mockContainerRemover) RemoveContainer(_ context.Context, containerID string) error {
	if m.err != nil {
		return m.err
	}
	m.removed = append(m.removed, containerID)
	return nil
}

func newTestReconciler(store state.Store, lister state.DockerLister) *state.Reconciler {
	lm := state.NewLockManager(5 * time.Second)
	logger := zap.NewNop()
	return state.NewReconciler(store, lm, lister, nil, 1*time.Hour, logger)
}

func newTestReconcilerWithRemover(store state.Store, lister state.DockerLister, remover state.ContainerRemover) *state.Reconciler {
	lm := state.NewLockManager(5 * time.Second)
	logger := zap.NewNop()
	return state.NewReconciler(store, lm, lister, remover, 1*time.Hour, logger)
}

func TestReconciler_TerminatesStateResourceNotInDocker(t *testing.T) {
	store := state.NewMemoryStore()
	ctx := context.Background()

	// State に ContainerID 付きのアクティブリソースを登録
	r := newResource("aws:ec2:instance", "res1", "aws", "ec2", "us-east-1", "active", nil)
	r.ContainerID = "container-abc123"
	store.Put(ctx, r)

	// Docker にはそのコンテナが存在しない
	lister := &mockDockerLister{containers: []container.Summary{}}
	rec := newTestReconciler(store, lister)

	if err := rec.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	got, err := store.Get(ctx, "aws:ec2:instance", "res1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Status != "terminated" {
		t.Errorf("expected status=terminated, got %q", got.Status)
	}
}

func TestReconciler_AddsOrphanForContainerNotInState(t *testing.T) {
	store := state.NewMemoryStore()
	ctx := context.Background()

	// Docker にはコンテナが存在するが State にはない
	cID := "abcdef123456789012" // 18 chars, will be truncated to 12
	lister := &mockDockerLister{
		containers: []container.Summary{
			{
				ID: cID,
				Labels: map[string]string{
					"cloudia.kind":     "aws:ec2:instance",
					"cloudia.provider": "aws",
					"cloudia.service":  "ec2",
					"cloudia.region":   "us-east-1",
				},
			},
		},
	}
	rec := newTestReconciler(store, lister)

	if err := rec.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// orphan として State に追加されているはず
	resourceID := cID[:12]
	got, err := store.Get(ctx, "aws:ec2:instance", resourceID)
	if err != nil {
		t.Fatalf("Get orphan failed: %v", err)
	}
	if got.Status != "orphan" {
		t.Errorf("expected status=orphan, got %q", got.Status)
	}
	if got.ContainerID != cID {
		t.Errorf("expected ContainerID=%q, got %q", cID, got.ContainerID)
	}
}

func TestReconciler_DockerErrorNoStateChange(t *testing.T) {
	store := state.NewMemoryStore()
	ctx := context.Background()

	// State にリソースを登録
	r := newResource("aws:ec2:instance", "res2", "aws", "ec2", "us-east-1", "active", nil)
	r.ContainerID = "container-xyz"
	store.Put(ctx, r)

	// Docker 接続エラー
	lister := &mockDockerLister{err: errors.New("docker: connection refused")}
	rec := newTestReconciler(store, lister)

	if err := rec.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile should not return error on Docker failure, got: %v", err)
	}

	// State は変更されていないはず
	got, err := store.Get(ctx, "aws:ec2:instance", "res2")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Status != "active" {
		t.Errorf("expected status=active (unchanged), got %q", got.Status)
	}
}

func TestReconciler_SkipsResourceWithoutContainerID(t *testing.T) {
	store := state.NewMemoryStore()
	ctx := context.Background()

	// ContainerID なしのリソース（Docker コンテナと無関係）
	r := newResource("docker:file:bucket", "res3", "docker", "file", "local", "active", nil)
	// ContainerID は空
	store.Put(ctx, r)

	lister := &mockDockerLister{containers: []container.Summary{}}
	rec := newTestReconciler(store, lister)

	if err := rec.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// ContainerID なしのリソースはそのまま
	got, err := store.Get(ctx, "docker:file:bucket", "res3")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Status != "active" {
		t.Errorf("expected status=active, got %q", got.Status)
	}
}

func TestReconciler_ExistingContainerInDockerNoChange(t *testing.T) {
	store := state.NewMemoryStore()
	ctx := context.Background()

	cID := "container-alive"
	r := newResource("aws:ec2:instance", "res4", "aws", "ec2", "us-east-1", "active", nil)
	r.ContainerID = cID
	store.Put(ctx, r)

	// Docker にも同じコンテナが存在する
	lister := &mockDockerLister{
		containers: []container.Summary{
			{ID: cID, Labels: map[string]string{}},
		},
	}
	rec := newTestReconciler(store, lister)

	if err := rec.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	got, err := store.Get(ctx, "aws:ec2:instance", "res4")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Status != "active" {
		t.Errorf("expected status=active (no change), got %q", got.Status)
	}
}

func TestReconciler_StartAndStop(t *testing.T) {
	store := state.NewMemoryStore()
	lister := &mockDockerLister{containers: []container.Summary{}}
	lm := state.NewLockManager(5 * time.Second)
	logger := zap.NewNop()

	rec := state.NewReconciler(store, lm, lister, nil, 10*time.Millisecond, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	rec.Start(ctx)
	<-ctx.Done()
	// goroutine が停止することを確認（パニックしないこと）
}

func TestReconciler_NoDeletesOnlyStatusUpdate(t *testing.T) {
	// remover=nil の場合、Reconciler はステータス変更のみ行う（Docker コンテナの削除なし）
	store := state.NewMemoryStore()
	ctx := context.Background()

	r := newResource("aws:ec2:instance", "res5", "aws", "ec2", "us-east-1", "active", nil)
	r.ContainerID = "container-gone"
	store.Put(ctx, r)

	lister := &mockDockerLister{containers: []container.Summary{}}
	rec := newTestReconciler(store, lister)

	if err := rec.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// リソースはまだ State に存在する（削除ではなくステータス変更のみ）
	got, err := store.Get(ctx, "aws:ec2:instance", "res5")
	if err != nil {
		t.Fatalf("resource should still exist in state: %v", err)
	}
	if got.Status != "terminated" {
		t.Errorf("expected status=terminated, got %q", got.Status)
	}
}

func TestReconciler_RemovesOrphanContainerWithRemover(t *testing.T) {
	store := state.NewMemoryStore()
	ctx := context.Background()

	cID := "orphan-container-abc"
	lister := &mockDockerLister{
		containers: []container.Summary{
			{
				ID: cID,
				Labels: map[string]string{
					"cloudia.kind":     "aws:ec2:instance",
					"cloudia.provider": "aws",
					"cloudia.service":  "ec2",
					"cloudia.region":   "us-east-1",
				},
			},
		},
	}
	remover := &mockContainerRemover{}
	rec := newTestReconcilerWithRemover(store, lister, remover)

	if err := rec.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// 孤立コンテナが削除されたことを確認
	if len(remover.removed) != 1 || remover.removed[0] != cID {
		t.Errorf("expected container %q to be removed, got %v", cID, remover.removed)
	}

	// State に orphan として追加されていることを確認
	resourceID := cID[:12]
	got, err := store.Get(ctx, "aws:ec2:instance", resourceID)
	if err != nil {
		t.Fatalf("Get orphan failed: %v", err)
	}
	if got.Status != "orphan" {
		t.Errorf("expected status=orphan, got %q", got.Status)
	}
}

func TestReconciler_OrphanCleanupLimit(t *testing.T) {
	store := state.NewMemoryStore()
	ctx := context.Background()

	// 11 件の孤立コンテナを用意（上限 10 件）
	containers := make([]container.Summary, 11)
	for i := range containers {
		containers[i] = container.Summary{
			ID: fmt.Sprintf("orphan-container-%02d-extra", i),
			Labels: map[string]string{
				"cloudia.kind":     "aws:ec2:instance",
				"cloudia.provider": "aws",
				"cloudia.service":  "ec2",
				"cloudia.region":   "us-east-1",
			},
		}
	}

	lister := &mockDockerLister{containers: containers}
	remover := &mockContainerRemover{}
	rec := newTestReconcilerWithRemover(store, lister, remover)

	if err := rec.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// 最大 10 件のみ削除されることを確認
	if len(remover.removed) != 10 {
		t.Errorf("expected 10 containers removed, got %d", len(remover.removed))
	}
}
