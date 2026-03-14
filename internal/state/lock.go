package state

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// LockManager はリソースごとの排他ロックを管理します。
// キーは "kind:id" 形式の文字列です。
type LockManager struct {
	mu             sync.Mutex
	locks          map[string]chan struct{}
	defaultTimeout time.Duration
}

// NewLockManager は指定したデフォルトタイムアウトで LockManager を返します。
func NewLockManager(defaultTimeout time.Duration) *LockManager {
	return &LockManager{
		locks:          make(map[string]chan struct{}),
		defaultTimeout: defaultTimeout,
	}
}

// Lock は kind と id で識別されるリソースのロックを取得します。
// ctx にデッドライン/タイムアウトが設定されていない場合は defaultTimeout を適用します。
// ロック取得前に ctx がキャンセルされた場合は ctx.Err() を返します。
func (m *LockManager) Lock(ctx context.Context, kind, id string) error {
	// ctx にデッドラインが設定されていない場合のみ defaultTimeout を適用する
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.defaultTimeout)
		defer cancel()
	}

	key := storeKey(kind, id)

	for {
		m.mu.Lock()
		ch, locked := m.locks[key]
		if !locked {
			// ロックを取得: チャネルを生成して map に登録
			m.locks[key] = make(chan struct{})
			m.mu.Unlock()
			return nil
		}
		m.mu.Unlock()

		// ロック解放を待つ
		select {
		case <-ch:
			// チャネルが close されたのでロック解放。再試行する
		case <-ctx.Done():
			return fmt.Errorf("lock %s/%s: %w", kind, id, ctx.Err())
		}
	}
}

// Unlock は kind と id で識別されるリソースのロックを解放します。
// ロックを保持していない場合は何もしません。
func (m *LockManager) Unlock(kind, id string) {
	key := storeKey(kind, id)

	m.mu.Lock()
	defer m.mu.Unlock()

	ch, locked := m.locks[key]
	if !locked {
		return
	}
	delete(m.locks, key)
	close(ch)
}

// TryLock はロックを即時取得しようとします。
// 取得できた場合は true、既にロックされている場合は false を返します。
func (m *LockManager) TryLock(kind, id string) bool {
	key := storeKey(kind, id)

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, locked := m.locks[key]; locked {
		return false
	}
	m.locks[key] = make(chan struct{})
	return true
}
