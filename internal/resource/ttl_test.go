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

func newTTLResource(kind, id, status string, ttl *time.Duration, createdAt time.Time) *models.Resource {
	return &models.Resource{
		Kind:      kind,
		ID:        id,
		Provider:  "aws",
		Service:   "ec2",
		Region:    "us-east-1",
		Status:    status,
		TTL:       ttl,
		CreatedAt: createdAt,
		UpdatedAt: time.Now(),
	}
}

func TestTTLManager_ExpiredResourceEventuallyTerminated(t *testing.T) {
	store := state.NewMemoryStore()
	ctx := context.Background()

	// TTL = 0、1秒前に作成なので即座に期限切れ
	// ContainerID 付き → Docker 削除が必要だが Docker は成功するのでそのまま削除される
	// テストでは「terminated になるか、CleanupOrphans で ErrNotFound になるか」を受け入れる
	ttl := time.Duration(0)
	r := newTTLResource("k", "expired1", "active", &ttl, time.Now().Add(-1*time.Second))
	store.Put(ctx, r)

	docker := &mockDockerRemover{}
	cleaner := NewCleaner(store, docker, zap.NewNop())

	shortInterval := 10 * time.Millisecond
	manager := NewTTLManager(store, cleaner, shortInterval, true, zap.NewNop())

	ctx2, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	manager.Start(ctx2)
	<-ctx2.Done()
	time.Sleep(10 * time.Millisecond)

	// TTL チェック後に CleanupOrphans も実行されるため
	// リソースが terminated になるか、既に削除されているかのどちらか
	got, err := store.Get(context.Background(), "k", "expired1")
	if err != nil {
		// CleanupOrphans によって削除済み = 期待通り
		if errors.Is(err, models.ErrNotFound) {
			return
		}
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != "terminated" {
		t.Errorf("expected status=terminated for expired resource, got %q", got.Status)
	}
}

func TestTTLManager_NonExpiredResourceUnchanged(t *testing.T) {
	store := state.NewMemoryStore()
	ctx := context.Background()

	// TTL = 1時間、作成直後なので期限切れではない
	ttl := 1 * time.Hour
	r := newTTLResource("k", "noexpiry1", "active", &ttl, time.Now())
	store.Put(ctx, r)

	docker := &mockDockerRemover{}
	cleaner := NewCleaner(store, docker, zap.NewNop())

	shortInterval := 10 * time.Millisecond
	manager := NewTTLManager(store, cleaner, shortInterval, true, zap.NewNop())

	ctx2, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	manager.Start(ctx2)
	<-ctx2.Done()
	time.Sleep(10 * time.Millisecond)

	got, err := store.Get(context.Background(), "k", "noexpiry1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Status != "active" {
		t.Errorf("expected status=active (not expired), got %q", got.Status)
	}
}

func TestTTLManager_NilTTLResourcePersists(t *testing.T) {
	store := state.NewMemoryStore()
	ctx := context.Background()

	// TTL = nil は永続リソース
	r := newTTLResource("k", "persist1", "active", nil, time.Now().Add(-24*time.Hour))
	store.Put(ctx, r)

	docker := &mockDockerRemover{}
	cleaner := NewCleaner(store, docker, zap.NewNop())

	shortInterval := 10 * time.Millisecond
	manager := NewTTLManager(store, cleaner, shortInterval, true, zap.NewNop())

	ctx2, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	manager.Start(ctx2)
	<-ctx2.Done()
	time.Sleep(10 * time.Millisecond)

	got, err := store.Get(context.Background(), "k", "persist1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Status != "active" {
		t.Errorf("expected status=active (nil TTL), got %q", got.Status)
	}
}

func TestTTLManager_ExpiredResourceEventuallyCleanedUp(t *testing.T) {
	store := state.NewMemoryStore()
	ctx := context.Background()

	// TTL 期限切れリソース（ContainerID なし）は TTL チェック後に Cleanup で State から削除される
	ttl := time.Duration(0)
	r := newTTLResource("k", "cleanup1", "active", &ttl, time.Now().Add(-1*time.Second))
	store.Put(ctx, r)

	docker := &mockDockerRemover{}
	cleaner := NewCleaner(store, docker, zap.NewNop())

	shortInterval := 10 * time.Millisecond
	manager := NewTTLManager(store, cleaner, shortInterval, true, zap.NewNop())

	ctx2, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	manager.Start(ctx2)
	<-ctx2.Done()
	time.Sleep(20 * time.Millisecond)

	// TTL 期限切れ → terminated → CleanupOrphans で State から削除
	_, err := store.Get(context.Background(), "k", "cleanup1")
	if !errors.Is(err, models.ErrNotFound) {
		t.Errorf("expected resource to be cleaned up (ErrNotFound), got: %v (status might be: checking)", err)
	}
}

func TestTTLManager_StartStopsOnContextCancel(t *testing.T) {
	store := state.NewMemoryStore()
	docker := &mockDockerRemover{}
	cleaner := NewCleaner(store, docker, zap.NewNop())
	manager := NewTTLManager(store, cleaner, 10*time.Millisecond, true, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	manager.Start(ctx)
	cancel()
	// パニックなく停止すること
	time.Sleep(20 * time.Millisecond)
}
