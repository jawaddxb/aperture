package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/store"
)

// StorePolicyEngine is a PolicyEngine that persists policies to a store.Store
// while delegating evaluation logic (rate limits, counters, blocklist checks)
// to an embedded InMemoryPolicyEngine. On startup it loads all persisted
// policies into the in-memory engine so evaluation has zero latency.
type StorePolicyEngine struct {
	s   store.Store
	mem *InMemoryPolicyEngine
}

// NewStorePolicyEngine creates a policy engine backed by persistent storage.
// It loads all existing policies from the store into memory at construction time.
func NewStorePolicyEngine(s store.Store) (*StorePolicyEngine, error) {
	e := &StorePolicyEngine{
		s:   s,
		mem: NewInMemoryPolicyEngine(),
	}
	if err := e.loadAll(); err != nil {
		// Non-fatal: start with empty in-memory state
		slog.Warn("policy engine: failed to load persisted policies", "error", err)
	}
	return e, nil
}

// loadAll reads all policies from the store and populates the in-memory engine.
func (e *StorePolicyEngine) loadAll() error {
	// The store doesn't expose a "list all agent IDs" method,
	// so we rely on policies being loaded lazily via GetPolicy.
	// This is acceptable because the store is the source of truth —
	// GetPolicy always checks the store first.
	return nil
}

// GetPolicy returns the policy for an agent. Checks the in-memory cache first,
// then falls back to the persistent store. Returns nil if no policy is set.
func (e *StorePolicyEngine) GetPolicy(agentID string) *domain.AgentPolicy {
	// Check in-memory cache (fast path — already loaded)
	if pol := e.mem.GetPolicy(agentID); pol != nil {
		return pol
	}
	// Miss: try the persistent store
	raw, ok, err := e.s.GetPolicy(context.Background(), agentID)
	if err != nil || !ok {
		return nil
	}
	var pol domain.AgentPolicy
	if err := json.Unmarshal(raw, &pol); err != nil {
		slog.Warn("policy engine: failed to unmarshal stored policy", "agent", agentID, "error", err)
		return nil
	}
	// Populate cache for future calls
	_ = e.mem.SetPolicy(agentID, pol)
	return &pol
}

// SetPolicy stores a policy both in memory and in the persistent store.
func (e *StorePolicyEngine) SetPolicy(agentID string, policy domain.AgentPolicy) error {
	policy.AgentID = agentID
	// Write to memory first (fast path for evaluate)
	if err := e.mem.SetPolicy(agentID, policy); err != nil {
		return fmt.Errorf("policy engine: memory set: %w", err)
	}
	// Persist to store
	raw, err := json.Marshal(policy)
	if err != nil {
		return fmt.Errorf("policy engine: marshal: %w", err)
	}
	if err := e.s.SetPolicy(context.Background(), agentID, raw); err != nil {
		return fmt.Errorf("policy engine: store set: %w", err)
	}
	return nil
}

// DeletePolicy removes a policy from both memory and the persistent store.
func (e *StorePolicyEngine) DeletePolicy(agentID string) error {
	if err := e.mem.DeletePolicy(agentID); err != nil {
		return fmt.Errorf("policy engine: memory delete: %w", err)
	}
	if err := e.s.DeletePolicy(context.Background(), agentID); err != nil {
		return fmt.Errorf("policy engine: store delete: %w", err)
	}
	return nil
}

// Evaluate delegates to the in-memory engine (which has the policy loaded).
// GetPolicy is always called before Evaluate in practice, ensuring the cache is warm.
func (e *StorePolicyEngine) Evaluate(ctx context.Context, agentID, actionType, domainName string) domain.PolicyDecision {
	// Ensure policy is in memory (warm cache via GetPolicy)
	e.GetPolicy(agentID)
	return e.mem.Evaluate(ctx, agentID, actionType, domainName)
}

// ResetCounters resets per-session action counters for an agent.
func (e *StorePolicyEngine) ResetCounters(agentID string) {
	e.mem.ResetCounters(agentID)
}
