// Package memory provides KV store implementations for agent state.
// This file implements a SQLite-backed AgentStateStore via the store.Store interface.
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/store"
)

// StoreBackedKV implements domain.AgentStateStore using a persistent store.Store.
// Values are JSON-serialized before storage and deserialized on retrieval.
// Safe for concurrent use (SQLite WAL handles concurrency).
type StoreBackedKV struct {
	s store.Store
}

// NewStoreBackedKV wraps a store.Store as a domain.AgentStateStore.
func NewStoreBackedKV(s store.Store) *StoreBackedKV {
	return &StoreBackedKV{s: s}
}

// Set stores a key-value pair for the given agent, serializing the value to JSON.
func (k *StoreBackedKV) Set(ctx context.Context, agentID, key string, value interface{}) error {
	if agentID == "" || key == "" {
		return fmt.Errorf("agentID and key must be non-empty")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("store kv: marshal value: %w", err)
	}
	return k.s.SetKV(ctx, agentID, key, json.RawMessage(raw))
}

// Get retrieves a key-value pair for the given agent.
// Returns domain.ErrMemoryNotFound if the key does not exist.
func (k *StoreBackedKV) Get(ctx context.Context, agentID, key string) (*domain.MemoryEntry, error) {
	raw, ok, err := k.s.GetKV(ctx, agentID, key)
	if err != nil {
		return nil, fmt.Errorf("store kv: get: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("store kv: unmarshal: %w", err)
	}
	return &domain.MemoryEntry{
		Key:       key,
		Value:     value,
		UpdatedAt: time.Now(), // SQLite store doesn't expose updated_at yet
	}, nil
}

// List returns all key-value pairs for the given agent with an optional key prefix.
func (k *StoreBackedKV) List(ctx context.Context, agentID, prefix string) ([]domain.MemoryEntry, error) {
	all, err := k.s.ListKV(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("store kv: list: %w", err)
	}
	var entries []domain.MemoryEntry
	for key, raw := range all {
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			continue
		}
		var value interface{}
		if err := json.Unmarshal(raw, &value); err != nil {
			continue // skip malformed entries
		}
		entries = append(entries, domain.MemoryEntry{
			Key:       key,
			Value:     value,
			UpdatedAt: time.Now(),
		})
	}
	return entries, nil
}

// Delete removes a key-value pair for the given agent.
func (k *StoreBackedKV) Delete(ctx context.Context, agentID, key string) error {
	return k.s.DeleteKV(ctx, agentID, key)
}
