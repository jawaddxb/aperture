// Package session provides DefaultSessionManager, which manages browser
// automation sessions backed by an in-memory store.
package session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/google/uuid"
)

// Config holds constructor parameters for DefaultSessionManager.
type Config struct {
	// Pool is the browser pool used to acquire/release browser instances.
	// Required.
	Pool domain.BrowserPool

	// Planner decomposes goals into plans.
	// Required.
	Planner domain.Planner

	// Sequencer executes plans step-by-step.
	// Required.
	Sequencer domain.Sequencer

	// AuthPersistence, when set, enables saving/restoring cookies across sessions.
	// Optional.
	AuthPersistence domain.AuthPersistence

	// PolicyEngine, when set, gates every action against xBPP policies.
	// Optional.
	PolicyEngine domain.PolicyEngine

	// MaxConcurrent is the maximum number of active sessions allowed at once.
	// Defaults to 5 when zero.
	MaxConcurrent int
}

// DefaultSessionManager manages browser sessions in memory.
// Implements domain.SessionManager. Safe for concurrent use.
type DefaultSessionManager struct {
	pool          domain.BrowserPool
	planner       domain.Planner
	sequencer     domain.Sequencer
	authPersist   domain.AuthPersistence
	policyEngine  domain.PolicyEngine
	maxConcurrent int

	mu       sync.RWMutex
	sessions map[string]*domain.Session
	browsers map[string]domain.BrowserInstance // sessionID → instance
}

// NewDefaultSessionManager constructs a DefaultSessionManager.
// Panics if required dependencies are nil.
func NewDefaultSessionManager(cfg Config) *DefaultSessionManager {
	if cfg.Pool == nil {
		panic("session.Config.Pool is required")
	}
	if cfg.Planner == nil {
		panic("session.Config.Planner is required")
	}
	if cfg.Sequencer == nil {
		panic("session.Config.Sequencer is required")
	}
	max := cfg.MaxConcurrent
	if max <= 0 {
		max = 5
	}
	return &DefaultSessionManager{
		pool:          cfg.Pool,
		planner:       cfg.Planner,
		sequencer:     cfg.Sequencer,
		authPersist:   cfg.AuthPersistence,
		policyEngine:  cfg.PolicyEngine,
		maxConcurrent: max,
		sessions:      make(map[string]*domain.Session),
		browsers:      make(map[string]domain.BrowserInstance),
	}
}

// Create creates a new session and acquires a browser instance from the pool.
// Returns ErrConcurrentLimitExceeded when the active session count is at max.
func (m *DefaultSessionManager) Create(ctx context.Context, goal string, meta map[string]string) (*domain.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeSessions() >= m.maxConcurrent {
		return nil, domain.ErrConcurrentLimitExceeded
	}

	inst, err := m.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire browser: %w", err)
	}

	now := time.Now()
	md := make(map[string]string)
	for k, v := range meta {
		md[k] = v
	}
	s := &domain.Session{
		ID:        uuid.New().String(),
		Status:    "active",
		BrowserID: inst.ID(),
		Goal:      goal,
		Metadata:  md,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Restore previously saved cookies into this browser instance.
	if m.authPersist != nil {
		_ = m.authPersist.LoadCookies(ctx, s.ID, inst) // best-effort
	}

	m.sessions[s.ID] = s
	m.browsers[s.ID] = inst
	return s, nil
}

// Get retrieves a session by ID.
func (m *DefaultSessionManager) Get(_ context.Context, id string) (*domain.Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.sessions[id]
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	return s, nil
}

// List returns all stored sessions.
func (m *DefaultSessionManager) List(_ context.Context) ([]*domain.Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]*domain.Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s)
	}
	return out, nil
}

// Update persists changes to an existing session.
func (m *DefaultSessionManager) Update(_ context.Context, session *domain.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[session.ID]; !ok {
		return domain.ErrSessionNotFound
	}
	session.UpdatedAt = time.Now()
	m.sessions[session.ID] = session
	return nil
}

// Delete removes a session and releases its browser back to the pool.
func (m *DefaultSessionManager) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[id]; !ok {
		return domain.ErrSessionNotFound
	}

	if inst, ok := m.browsers[id]; ok {
		m.pool.Release(inst)
		delete(m.browsers, id)
	}
	delete(m.sessions, id)
	return nil
}

// Execute runs the full goal for the given session:
// plan → sequence → update session status and results.
func (m *DefaultSessionManager) Execute(ctx context.Context, id string) (*domain.RunResult, error) {
	inst, session, err := m.lockedLookup(id)
	if err != nil {
		return nil, err
	}

	plan, err := m.planner.Plan(ctx, session.Goal, nil)
	if err != nil {
		m.markFailed(id)
		return nil, fmt.Errorf("plan: %w", err)
	}

	m.setSessionPlan(id, plan)

	// xBPP policy gate: check each step against policy before execution.
	if m.policyEngine != nil {
		agentID := session.Metadata["agent_id"]
		if agentID == "" {
			agentID = session.ID
		}
		for _, step := range plan.Steps {
			currentDomain := ""
			if session.Plan != nil && len(session.Results) > 0 {
				last := session.Results[len(session.Results)-1]
				if last.Result != nil && last.Result.PageState != nil {
					currentDomain = last.Result.PageState.URL
				}
			}
			decision := m.policyEngine.Evaluate(ctx, agentID, step.Action, currentDomain)
			switch decision.Result {
			case domain.PolicyBlock:
				m.markFailed(id)
				return nil, fmt.Errorf("policy_blocked: %s", decision.Reason)
			case domain.PolicyEscalate:
				m.markFailed(id)
				return nil, fmt.Errorf("escalation_required: %s", decision.Reason)
			}
		}
	}

	result, err := m.sequencer.Run(ctx, inst, plan)
	if err != nil {
		m.markFailed(id)
		return nil, fmt.Errorf("execute: %w", err)
	}

	// Persist cookies for future sessions.
	if m.authPersist != nil {
		_ = m.authPersist.SaveCookies(ctx, id, inst) // best-effort
	}

	m.finaliseSession(id, result)
	return result, nil
}

// ─── internal helpers ─────────────────────────────────────────────────────────

// activeSessions returns the number of sessions with status="active".
// Caller must hold m.mu (any lock is sufficient).
func (m *DefaultSessionManager) activeSessions() int {
	count := 0
	for _, s := range m.sessions {
		if s.Status == "active" {
			count++
		}
	}
	return count
}

// lockedLookup safely retrieves the browser instance and session for id.
func (m *DefaultSessionManager) lockedLookup(id string) (domain.BrowserInstance, *domain.Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.sessions[id]
	if !ok {
		return nil, nil, domain.ErrSessionNotFound
	}
	inst, ok := m.browsers[id]
	if !ok {
		return nil, nil, fmt.Errorf("no browser for session %s", id)
	}
	return inst, s, nil
}

// setSessionPlan stores the plan in the session under a write lock.
func (m *DefaultSessionManager) setSessionPlan(id string, plan *domain.Plan) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[id]; ok {
		s.Plan = plan
		s.UpdatedAt = time.Now()
	}
}

// markFailed updates the session status to "failed".
func (m *DefaultSessionManager) markFailed(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[id]; ok {
		s.Status = "failed"
		s.UpdatedAt = time.Now()
	}
}

// finaliseSession stores results and sets status based on RunResult.
func (m *DefaultSessionManager) finaliseSession(id string, result *domain.RunResult) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[id]
	if !ok {
		return
	}

	s.Results = make([]*domain.StepResult, len(result.Steps))
	for i := range result.Steps {
		sr := result.Steps[i]
		s.Results[i] = &sr
	}

	if result.Success {
		s.Status = "completed"
	} else {
		s.Status = "failed"
	}
	s.UpdatedAt = time.Now()
}

// compile-time interface assertion.
var _ domain.SessionManager = (*DefaultSessionManager)(nil)
