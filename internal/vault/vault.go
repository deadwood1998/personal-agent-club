// Package vault keeps account credentials. Credentials go in and are read back only by the core to call a
// provider; no protocol method ever returns one. File keeps a space's credentials encrypted (AES-256-GCM) under a key
// kept in its own file; Memory is for tests.
package vault

import (
	"context"
	"fmt"
	"sync"
)

// Vault keeps one credential per account id.
type Vault interface {
	Put(ctx context.Context, accountID, credential string) error
	// Get is the account's credential; ok is false when there is none.
	Get(ctx context.Context, accountID string) (credential string, ok bool, err error)
	Delete(ctx context.Context, accountID string) error
}

// Memory is a Vault in memory. It formats without its contents, so logging it never leaks a credential.
type Memory struct {
	mu    sync.Mutex
	creds map[string]string
}

// NewMemory is an empty in-memory vault.
func NewMemory() *Memory { return &Memory{creds: map[string]string{}} }

func (m *Memory) Put(_ context.Context, accountID, credential string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.creds[accountID] = credential
	return nil
}

func (m *Memory) Get(_ context.Context, accountID string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.creds[accountID]
	return c, ok, nil
}

func (m *Memory) Delete(_ context.Context, accountID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.creds, accountID)
	return nil
}

func (m *Memory) String() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return fmt.Sprintf("vault.Memory(%d credentials)", len(m.creds))
}

// GoString keeps %#v from printing the map.
func (m *Memory) GoString() string { return m.String() }
