package state

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/HMasataka/cloudia/pkg/models"
)

// MemoryStore はインメモリの Store 実装です。
// キーは "kind:id" 形式の文字列です。
type MemoryStore struct {
	mu   sync.RWMutex
	data map[string]*models.Resource
}

// NewMemoryStore は空の MemoryStore を返します。
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: make(map[string]*models.Resource),
	}
}

func storeKey(kind, id string) string {
	return kind + ":" + id
}

// Get は kind と id でリソースを取得します。見つからない場合は models.ErrNotFound を返します。
func (m *MemoryStore) Get(_ context.Context, kind, id string) (*models.Resource, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	r, ok := m.data[storeKey(kind, id)]
	if !ok {
		return nil, fmt.Errorf("resource %s/%s: %w", kind, id, models.ErrNotFound)
	}
	// シャローコピーを返して外部変更を防ぐ
	cp := *r
	return &cp, nil
}

// List は kind に一致するリソースを Filter で絞り込んで返します。
// kind が空文字の場合は全 Kind を対象にします。
func (m *MemoryStore) List(_ context.Context, kind string, filter Filter) ([]*models.Resource, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*models.Resource
	for _, r := range m.data {
		if kind != "" && r.Kind != kind {
			continue
		}
		if !matchFilter(r, filter) {
			continue
		}
		cp := *r
		result = append(result, &cp)
	}
	return result, nil
}

// matchFilter は Filter の全条件が Resource に一致するかを判定します。
func matchFilter(r *models.Resource, filter Filter) bool {
	for k, v := range filter {
		switch k {
		case "Status":
			if r.Status != v {
				return false
			}
		case "Provider":
			if r.Provider != v {
				return false
			}
		case "Service":
			if r.Service != v {
				return false
			}
		case "Region":
			if r.Region != v {
				return false
			}
		case "Kind":
			if r.Kind != v {
				return false
			}
		default:
			if strings.HasPrefix(k, "tag:") {
				tagKey := strings.TrimPrefix(k, "tag:")
				if r.Tags == nil || r.Tags[tagKey] != v {
					return false
				}
			} else if strings.HasPrefix(k, "spec:") {
				specKey := strings.TrimPrefix(k, "spec:")
				if r.Spec == nil {
					return false
				}
				specVal, ok := r.Spec[specKey]
				if !ok {
					return false
				}
				if fmt.Sprintf("%v", specVal) != v {
					return false
				}
			}
		}
	}
	return true
}

// Put はリソースを保存します。同一 kind:id が存在する場合は上書きします。
func (m *MemoryStore) Put(_ context.Context, resource *models.Resource) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cp := *resource
	m.data[storeKey(resource.Kind, resource.ID)] = &cp
	return nil
}

// Delete は kind と id でリソースを削除します。
func (m *MemoryStore) Delete(_ context.Context, kind, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.data, storeKey(kind, id))
	return nil
}

// Snapshot は現在の State をファイルにアトミック書き込みします。
// RLock → データコピー → RLock 解放 → JSON Marshal → temp + rename の順で処理します。
func (m *MemoryStore) Snapshot(_ context.Context, path string) error {
	m.mu.RLock()
	snapshot := make(map[string]*models.Resource, len(m.data))
	for k, v := range m.data {
		cp := *v
		snapshot[k] = &cp
	}
	m.mu.RUnlock()

	b, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("snapshot marshal: %w", err)
	}

	return atomicWrite(path, b)
}

// Restore はファイルから State を読み込み、現在の map を置き換えます。
func (m *MemoryStore) Restore(_ context.Context, path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("restore read file: %w", err)
	}

	var loaded map[string]*models.Resource
	if err := json.Unmarshal(b, &loaded); err != nil {
		return fmt.Errorf("restore unmarshal: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = loaded
	return nil
}

// atomicWrite はデータを一時ファイルに書き込んでからリネームします。
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("atomic write create temp: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("atomic write write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("atomic write close: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("atomic write rename: %w", err)
	}
	return nil
}
