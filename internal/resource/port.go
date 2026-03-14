package resource

import (
	"fmt"
	"sync"

	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/pkg/models"
)

// PortManager はポートの割り当てと解放を管理します。
type PortManager struct {
	mu             sync.Mutex
	ports          map[int]string // port -> resourceID
	resourcePorts  map[string]int // resourceID -> allocated port count
	rangeStart     int
	rangeEnd       int
	maxPerResource int
}

// NewPortManager は PortConfig から PortManager を生成します。
func NewPortManager(cfg config.PortConfig) *PortManager {
	return &PortManager{
		ports:          make(map[int]string),
		resourcePorts:  make(map[string]int),
		rangeStart:     cfg.RangeStart,
		rangeEnd:       cfg.RangeEnd,
		maxPerResource: cfg.MaxPerResource,
	}
}

// Allocate は preferred ポートを resourceID に割り当てます。
// preferred が使用中の場合は preferred+1 から線形探索でフォールバックします。
// ポートが枯渇した場合は models.ErrLimitExceeded を返します。
func (m *PortManager) Allocate(preferred int, resourceID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.checkResourceLimit(resourceID); err != nil {
		return 0, err
	}

	// preferred ポートが空いていれば即座に割り当て
	if _, used := m.ports[preferred]; !used && m.inRange(preferred) {
		m.assign(preferred, resourceID)
		return preferred, nil
	}

	// preferred+1 から線形探索
	start := preferred + 1
	for port := start; port <= m.rangeEnd; port++ {
		if _, used := m.ports[port]; !used {
			m.assign(port, resourceID)
			return port, nil
		}
	}
	// rangeStart から preferred まで探索
	for port := m.rangeStart; port < preferred; port++ {
		if _, used := m.ports[port]; !used {
			m.assign(port, resourceID)
			return port, nil
		}
	}

	return 0, fmt.Errorf("all ports exhausted in range [%d, %d]: %w", m.rangeStart, m.rangeEnd, models.ErrLimitExceeded)
}

// AllocateAny は rangeStart から空きポートを探して resourceID に割り当てます。
// ポートが枯渇した場合は models.ErrLimitExceeded を返します。
func (m *PortManager) AllocateAny(resourceID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.checkResourceLimit(resourceID); err != nil {
		return 0, err
	}

	for port := m.rangeStart; port <= m.rangeEnd; port++ {
		if _, used := m.ports[port]; !used {
			m.assign(port, resourceID)
			return port, nil
		}
	}

	return 0, fmt.Errorf("all ports exhausted in range [%d, %d]: %w", m.rangeStart, m.rangeEnd, models.ErrLimitExceeded)
}

// Release は指定ポートの割り当てを解放します。
// 割り当てられていないポートは無視します。
func (m *PortManager) Release(port int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	resourceID, ok := m.ports[port]
	if !ok {
		return
	}

	delete(m.ports, port)
	m.resourcePorts[resourceID]--
	if m.resourcePorts[resourceID] <= 0 {
		delete(m.resourcePorts, resourceID)
	}
}

// IsAvailable は指定ポートが割り当て可能かどうかを返します。
func (m *PortManager) IsAvailable(port int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, used := m.ports[port]
	return !used && m.inRange(port)
}

// checkResourceLimit は resourceID のポート割り当て数が上限に達しているか確認します。
// ロックは呼び出し元が保持していることを前提とします。
func (m *PortManager) checkResourceLimit(resourceID string) error {
	count := m.resourcePorts[resourceID]
	if m.maxPerResource > 0 && count >= m.maxPerResource {
		return fmt.Errorf("resource %q has reached max port limit %d: %w", resourceID, m.maxPerResource, models.ErrLimitExceeded)
	}
	return nil
}

// assign はロックを保持した状態でポートを割り当てます。
func (m *PortManager) assign(port int, resourceID string) {
	m.ports[port] = resourceID
	m.resourcePorts[resourceID]++
}

// inRange はポートが管理範囲内かどうかを返します。
func (m *PortManager) inRange(port int) bool {
	return port >= m.rangeStart && port <= m.rangeEnd
}
