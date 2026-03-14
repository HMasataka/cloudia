package state_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/HMasataka/cloudia/internal/state"
)

func TestLockManager_LockUnlock(t *testing.T) {
	lm := state.NewLockManager(5 * time.Second)

	if err := lm.Lock(context.Background(), "aws:s3:bucket", "res1"); err != nil {
		t.Fatalf("Lock failed: %v", err)
	}
	lm.Unlock("aws:s3:bucket", "res1")
}

func TestLockManager_LockTimeout(t *testing.T) {
	lm := state.NewLockManager(5 * time.Second)

	// goroutine A がロック取得
	if err := lm.Lock(context.Background(), "aws:s3:bucket", "res1"); err != nil {
		t.Fatalf("first Lock failed: %v", err)
	}

	// goroutine B がタイムアウト付き ctx でロック待ち → DeadlineExceeded を期待
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := lm.Lock(ctx, "aws:s3:bucket", "res1")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}

	lm.Unlock("aws:s3:bucket", "res1")
}

func TestLockManager_DefaultTimeout(t *testing.T) {
	// defaultTimeout が短い LockManager でタイムアウトを確認
	lm := state.NewLockManager(100 * time.Millisecond)

	if err := lm.Lock(context.Background(), "k", "id"); err != nil {
		t.Fatalf("first Lock failed: %v", err)
	}

	// ctx にデッドラインなし → defaultTimeout が適用されて DeadlineExceeded
	err := lm.Lock(context.Background(), "k", "id")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded from defaultTimeout, got %v", err)
	}

	lm.Unlock("k", "id")
}

func TestLockManager_IndependentKeys(t *testing.T) {
	lm := state.NewLockManager(5 * time.Second)

	// kind:id が異なるロックは独立して動作する
	if err := lm.Lock(context.Background(), "k1", "id1"); err != nil {
		t.Fatalf("Lock k1/id1 failed: %v", err)
	}
	if err := lm.Lock(context.Background(), "k2", "id2"); err != nil {
		t.Fatalf("Lock k2/id2 failed: %v", err)
	}

	lm.Unlock("k1", "id1")
	lm.Unlock("k2", "id2")
}

func TestLockManager_TryLock(t *testing.T) {
	lm := state.NewLockManager(5 * time.Second)

	// 未ロック状態では true
	if !lm.TryLock("k", "id") {
		t.Fatal("TryLock should succeed on unlocked resource")
	}

	// ロック中は false
	if lm.TryLock("k", "id") {
		t.Fatal("TryLock should fail on locked resource")
	}

	lm.Unlock("k", "id")

	// 解放後は再び true
	if !lm.TryLock("k", "id") {
		t.Fatal("TryLock should succeed after Unlock")
	}
	lm.Unlock("k", "id")
}

func TestLockManager_ConcurrentAccess(t *testing.T) {
	lm := state.NewLockManager(5 * time.Second)
	var wg sync.WaitGroup
	counter := 0

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := lm.Lock(context.Background(), "k", "id"); err != nil {
				return
			}
			counter++
			lm.Unlock("k", "id")
		}()
	}
	wg.Wait()

	if counter != 50 {
		t.Fatalf("expected counter=50, got %d", counter)
	}
}
