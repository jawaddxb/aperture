// Package domain defines core interfaces for Aperture.
// This file defines the AgentStateStore interface for agent KV memory.
package domain

import (
	"context"
	"time"
)

// MemoryEntry is a single key-value pair in an agent's state store.
type MemoryEntry struct {
	Key       string      `json:"key"`
	Value     interface{} `json:"value"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// AgentStateStore provides per-agent key-value state persistence.
type AgentStateStore interface {
	Set(ctx context.Context, agentID, key string, value interface{}) error
	Get(ctx context.Context, agentID, key string) (*MemoryEntry, error)
	List(ctx context.Context, agentID, prefix string) ([]MemoryEntry, error)
	Delete(ctx context.Context, agentID, key string) error
}
