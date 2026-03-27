// Package executor implements browser action executors for Aperture.
// This file provides the PauseExecutor (human-in-the-loop) and DefaultHITLManager.
package executor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// hitlDefaultTimeout is the default time PauseExecutor will wait for a human response.
const hitlDefaultTimeout = 10 * time.Minute

// errNoSuchIntervention is returned when an ID is not found in the wait queue.
var errNoSuchIntervention = errors.New("no pending intervention with that ID")

// errInterventionCancelled is returned when CancelIntervention is called.
var errInterventionCancelled = errors.New("intervention cancelled")

// ─── DefaultHITLManager ──────────────────────────────────────────────────────

// pendingEntry holds the response channel for one pending intervention.
type pendingEntry struct {
	ch chan *domain.InterventionResponse
}

// DefaultHITLManager is an in-memory HITLManager.
// Callers blocked in RequestIntervention wait on a per-request channel that is
// written by ResolveIntervention or closed by CancelIntervention.
// Safe for concurrent use.
type DefaultHITLManager struct {
	mu      sync.Mutex
	pending map[string]*pendingEntry
}

// NewDefaultHITLManager constructs a DefaultHITLManager.
func NewDefaultHITLManager() *DefaultHITLManager {
	return &DefaultHITLManager{
		pending: make(map[string]*pendingEntry),
	}
}

// RequestIntervention registers req in the wait queue and blocks until it is
// resolved, cancelled, or the context expires.
func (m *DefaultHITLManager) RequestIntervention(
	ctx context.Context,
	req *domain.InterventionRequest,
) (*domain.InterventionResponse, error) {
	ch := make(chan *domain.InterventionResponse, 1)

	m.mu.Lock()
	m.pending[req.ID] = &pendingEntry{ch: ch}
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.pending, req.ID)
		m.mu.Unlock()
	}()

	select {
	case resp, ok := <-ch:
		if !ok {
			return nil, errInterventionCancelled
		}
		return resp, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("intervention %s timed out: %w", req.ID, ctx.Err())
	}
}

// ResolveIntervention sends resp to the goroutine waiting on id.
// Returns errNoSuchIntervention when id is not pending.
func (m *DefaultHITLManager) ResolveIntervention(
	_ context.Context,
	id string,
	resp *domain.InterventionResponse,
) error {
	m.mu.Lock()
	entry, ok := m.pending[id]
	m.mu.Unlock()

	if !ok {
		return errNoSuchIntervention
	}
	entry.ch <- resp
	return nil
}

// CancelIntervention closes the channel for id, causing RequestIntervention to
// return errInterventionCancelled.
// Returns errNoSuchIntervention when id is not pending.
func (m *DefaultHITLManager) CancelIntervention(_ context.Context, id string) error {
	m.mu.Lock()
	entry, ok := m.pending[id]
	if ok {
		delete(m.pending, id)
	}
	m.mu.Unlock()

	if !ok {
		return errNoSuchIntervention
	}
	close(entry.ch)
	return nil
}

// ─── PauseExecutor ───────────────────────────────────────────────────────────

// PauseExecutor pauses workflow execution and requests human intervention.
// It implements domain.Executor.
type PauseExecutor struct {
	hitl domain.HITLManager
}

// NewPauseExecutor constructs a PauseExecutor backed by the given HITLManager.
func NewPauseExecutor(hitl domain.HITLManager) *PauseExecutor {
	return &PauseExecutor{hitl: hitl}
}

// Execute pauses execution and waits for human intervention.
// Implements domain.Executor.
func (e *PauseExecutor) Execute(
	ctx context.Context,
	inst domain.BrowserInstance,
	params map[string]interface{},
) (*domain.ActionResult, error) {
	start := time.Now()
	result := &domain.ActionResult{Action: "pause"}

	req, timeout, err := e.buildRequest(ctx, inst, params)
	if err != nil {
		return failResult(result, start, fmt.Errorf("pause: %w", err)), nil
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp, err := e.hitl.RequestIntervention(waitCtx, req)
	if err != nil {
		return failResult(result, start, fmt.Errorf("pause: intervention failed: %w", err)), nil
	}
	if !resp.Success {
		return failResult(result, start, fmt.Errorf("pause: intervention not successful")), nil
	}

	result.Success = true
	result.Data = []byte(resp.Data)
	result.Duration = time.Since(start)
	return result, nil
}

// buildRequest parses params and prepares the intervention request.
func (e *PauseExecutor) buildRequest(ctx context.Context, inst domain.BrowserInstance, params map[string]interface{}) (*domain.InterventionRequest, time.Duration, error) {
	itype, err := stringParam(params, "type")
	if err != nil {
		return nil, 0, err
	}
	prompt, err := stringParam(params, "prompt")
	if err != nil {
		return nil, 0, err
	}

	sessionID, _ := params["sessionID"].(string)
	timeout := hitlDefaultTimeout
	if v, ok := params["timeout"].(time.Duration); ok {
		timeout = v
	}

	req := &domain.InterventionRequest{
		ID:         uuid.New().String(),
		SessionID:  sessionID,
		Type:       itype,
		Prompt:     prompt,
		Screenshot: takeScreenshotBytes(ctx, inst),
		CreatedAt:  time.Now().UTC(),
	}
	return req, timeout, nil
}

// takeScreenshotBytes captures a PNG screenshot and returns the raw bytes.
// Returns nil on failure (best-effort; callers must handle nil).
func takeScreenshotBytes(ctx context.Context, inst domain.BrowserInstance) []byte {
	se := NewScreenshotExecutor()
	r, err := se.Execute(ctx, inst, map[string]interface{}{"format": "png"})
	if err != nil || !r.Success {
		return nil
	}
	return r.Data
}
