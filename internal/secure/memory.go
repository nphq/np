package secure

import (
	"sync"
)

// MemoryKeyring 是测试用的内存实现（不落盘）。
type MemoryKeyring struct {
	mu     sync.Mutex
	tokens map[string]string
}

// NewMemory 创建测试用内存 Keyring。
func NewMemory() *MemoryKeyring {
	return &MemoryKeyring{tokens: map[string]string{}}
}

func (m *MemoryKeyring) SaveToken(clusterID, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens[clusterID] = token
	return nil
}

func (m *MemoryKeyring) GetToken(clusterID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tokens[clusterID]
	if !ok {
		return "", ErrTokenNotFound
	}
	return t, nil
}

func (m *MemoryKeyring) DeleteToken(clusterID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.tokens[clusterID]; !ok {
		return ErrTokenNotFound
	}
	delete(m.tokens, clusterID)
	return nil
}

// compile-time guard
var _ Keyring = (*MemoryKeyring)(nil)
var _ Keyring = (*OSKeyring)(nil)
