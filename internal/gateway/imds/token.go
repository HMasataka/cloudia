package imds

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// tokenEntry はトークンと有効期限を保持します。
type tokenEntry struct {
	expiresAt time.Time
}

// TokenStore はIMDSv2トークンを管理します。
type TokenStore struct {
	mu     sync.RWMutex
	tokens map[string]tokenEntry
}

// NewTokenStore は TokenStore を生成します。
func NewTokenStore() *TokenStore {
	return &TokenStore{
		tokens: make(map[string]tokenEntry),
	}
}

// Generate はランダムトークンを生成し、TTL付きで保存します。
func (s *TokenStore) Generate(ttl time.Duration) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("imds: failed to generate token: %w", err)
	}
	token := hex.EncodeToString(b)

	s.mu.Lock()
	s.tokens[token] = tokenEntry{expiresAt: time.Now().Add(ttl)}
	s.mu.Unlock()

	return token, nil
}

// Validate はトークンが有効かどうかを検証します。
// トークンが存在しない場合は false を返します。
// 有効期限切れの場合は false を返します。
func (s *TokenStore) Validate(token string) bool {
	s.mu.RLock()
	entry, ok := s.tokens[token]
	s.mu.RUnlock()

	if !ok {
		return false
	}
	return time.Now().Before(entry.expiresAt)
}
