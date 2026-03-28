// Package memory provides an in-memory key-value store for agent state.
package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// InMemoryKV is a simple in-memory key-value store for agent state.
// Implements domain.AgentStateStore. Safe for concurrent use.
type InMemoryKV struct {
	mu   sync.RWMutex
	data map[string]map[string]domain.MemoryEntry // agentID → key → entry
}

// NewInMemoryKV constructs a ready-to-use in-memory KV store.
func NewInMemoryKV() *InMemoryKV {
	return &InMemoryKV{data: make(map[string]map[string]domain.MemoryEntry)}
}

// Set stores or updates a key-value pair for the given agent.
func (k *InMemoryKV) Set(_ context.Context, agentID, key string, value interface{}) error {
	if agentID == "" || key == "" {
		return fmt.Errorf("agentID and key must be non-empty")
	}
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.data[agentID] == nil {
		k.data[agentID] = make(map[string]domain.MemoryEntry)
	}
	k.data[agentID][key] = domain.MemoryEntry{
		Key:       key,
		Value:     value,
		UpdatedAt: time.Now(),
	}
	return nil
}

// Get retrieves a single key for the given agent.
func (k *InMemoryKV) Get(_ context.Context, agentID, key string) (*domain.MemoryEntry, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()

	agent, ok := k.data[agentID]
	if !ok {
		return nil, nil
	}
	entry, ok := agent[key]
	if !ok {
		return nil, nil
	}
	return &entry, nil
}

// List returns all entries for the given agent, optionally filtered by key prefix.
func (k *InMemoryKV) List(_ context.Context, agentID, prefix string) ([]domain.MemoryEntry, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()

	agent, ok := k.data[agentID]
	if !ok {
		return []domain.MemoryEntry{}, nil
	}

	var entries []domain.MemoryEntry
	for key, entry := range agent {
		if prefix == "" || strings.HasPrefix(key, prefix) {
			entries = append(entries, entry)
		}
	}
	if entries == nil {
		entries = []domain.MemoryEntry{}
	}
	return entries, nil
}

// Delete removes a key for the given agent.
func (k *InMemoryKV) Delete(_ context.Context, agentID, key string) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	agent, ok := k.data[agentID]
	if !ok {
		return nil
	}
	delete(agent, key)
	return nil
}

// compile-time interface assertion.
var _ domain.AgentStateStore = (*InMemoryKV)(nil)
