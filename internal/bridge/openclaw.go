// Package bridge provides the OpenClawBridge, which exposes Aperture's
// browser-automation capabilities as a structured task execution interface
// for the OpenClaw integration layer.
package bridge

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/google/uuid"
)

// defaultTimeout is used when TaskConfig.Timeout is zero.
const defaultTimeout = 120 * time.Second

// defaultMaxConcurrent is the maximum number of concurrent tasks.
const defaultMaxConcurrent = 10

// runningTask holds live state for an in-progress task.
type runningTask struct {
	req      *domain.TaskRequest
	response *domain.TaskResponse
	cancel   context.CancelFunc
	done     chan struct{}
}

// Config holds constructor parameters for OpenClawBridge.
type Config struct {
	// SessionManager is used to create and execute automation sessions.
	// Required.
	SessionManager domain.SessionManager

	// MaxConcurrent limits how many tasks may run simultaneously.
	// Defaults to defaultMaxConcurrent when zero.
	MaxConcurrent int
}

// OpenClawBridge implements domain.Bridge using the session manager pipeline.
// It tracks concurrent tasks and supports cancellation.
// Safe for concurrent use.
type OpenClawBridge struct {
	sessions      domain.SessionManager
	maxConcurrent int

	mu    sync.RWMutex
	tasks map[string]*runningTask
}

// NewOpenClawBridge constructs an OpenClawBridge from cfg.
// Panics if SessionManager is nil.
func NewOpenClawBridge(cfg Config) *OpenClawBridge {
	if cfg.SessionManager == nil {
		panic("bridge.Config.SessionManager is required")
	}
	max := cfg.MaxConcurrent
	if max <= 0 {
		max = defaultMaxConcurrent
	}
	return &OpenClawBridge{
		sessions:      cfg.SessionManager,
		maxConcurrent: max,
		tasks:         make(map[string]*runningTask),
	}
}

// ExecuteTask runs req synchronously: create session → navigate → plan →
// execute → capture final state → return TaskResponse.
// Blocks until completion or ctx cancellation.
func (b *OpenClawBridge) ExecuteTask(ctx context.Context, req *domain.TaskRequest) (*domain.TaskResponse, error) {
	if err := b.checkConcurrentLimit(); err != nil {
		return nil, err
	}

	id := req.ID
	if id == "" {
		id = uuid.New().String()
	}

	timeout := resolveTimeout(req.Config.Timeout)
	taskCtx, cancel := context.WithTimeout(ctx, timeout)

	task := &runningTask{
		req:    req,
		cancel: cancel,
		done:   make(chan struct{}),
	}

	b.registerTask(id, task)
	defer b.finishTask(id, cancel)

	resp := b.runTask(taskCtx, id, req)
	task.response = resp
	return resp, nil
}

// GetStatus returns the result of a completed task or its in-progress state.
func (b *OpenClawBridge) GetStatus(_ context.Context, taskID string) (*domain.TaskResponse, error) {
	b.mu.RLock()
	task, ok := b.tasks[taskID]
	b.mu.RUnlock()

	if !ok {
		return nil, domain.ErrTaskNotFound
	}

	select {
	case <-task.done:
		return task.response, nil
	default:
		return &domain.TaskResponse{
			ID:    taskID,
			Goal:  task.req.Goal,
			Error: "task still running",
		}, nil
	}
}

// CancelTask cancels a running task by its ID.
func (b *OpenClawBridge) CancelTask(_ context.Context, taskID string) error {
	b.mu.RLock()
	task, ok := b.tasks[taskID]
	b.mu.RUnlock()

	if !ok {
		return domain.ErrTaskNotFound
	}

	task.cancel()
	return nil
}

// ActiveTaskCount returns the number of currently tracked tasks.
func (b *OpenClawBridge) ActiveTaskCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.tasks)
}

// ─── internal helpers ─────────────────────────────────────────────────────────

// checkConcurrentLimit returns an error if the concurrent limit is reached.
func (b *OpenClawBridge) checkConcurrentLimit() error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if len(b.tasks) >= b.maxConcurrent {
		return fmt.Errorf("bridge: concurrent task limit of %d reached", b.maxConcurrent)
	}
	return nil
}

// registerTask adds task to the in-flight map under id.
func (b *OpenClawBridge) registerTask(id string, task *runningTask) {
	b.mu.Lock()
	b.tasks[id] = task
	b.mu.Unlock()
}

// finishTask cancels the context, closes the done channel, and removes the
// task from the map.
func (b *OpenClawBridge) finishTask(id string, cancel context.CancelFunc) {
	cancel()
	b.mu.Lock()
	if task, ok := b.tasks[id]; ok {
		select {
		case <-task.done:
		default:
			close(task.done)
		}
	}
	delete(b.tasks, id)
	b.mu.Unlock()
}

// runTask orchestrates session creation and execution, returning a TaskResponse.
func (b *OpenClawBridge) runTask(ctx context.Context, id string, req *domain.TaskRequest) *domain.TaskResponse {
	start := time.Now()
	resp := &domain.TaskResponse{
		ID:   id,
		Goal: req.Goal,
	}

	session, err := b.sessions.Create(ctx, req.Goal, nil)
	if err != nil {
		resp.Error = fmt.Sprintf("create session: %s", err)
		resp.Duration = time.Since(start)
		return resp
	}

	defer func() {
		_ = b.sessions.Delete(context.Background(), session.ID)
	}()

	if req.URL != "" {
		if navErr := b.navigateSession(ctx, session.ID, req.URL); navErr != nil {
			resp.Error = fmt.Sprintf("navigate: %s", navErr)
			resp.Duration = time.Since(start)
			return resp
		}
	}

	result, err := b.sessions.Execute(ctx, session.ID)
	if err != nil {
		resp.Error = fmt.Sprintf("execute: %s", err)
		resp.Duration = time.Since(start)
		return resp
	}

	resp.Success = result.Success
	resp.Steps = mapStepResults(result.Steps)
	resp.FinalURL = extractFinalURL(result.Steps)
	resp.FinalTitle = extractFinalTitle(result.Steps)
	resp.Duration = time.Since(start)
	return resp
}

// navigateSession injects a navigate step before the main plan by updating
// the session goal to include the URL. This is a best-effort approach using
// the existing session manager — callers can also set URL in the goal directly.
// We log it but do not fail hard on errors here since the main execute will
// run regardless.
func (b *OpenClawBridge) navigateSession(ctx context.Context, sessionID, url string) error {
	s, err := b.sessions.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	s.Goal = fmt.Sprintf("navigate to %s then %s", url, s.Goal)
	return b.sessions.Update(ctx, s)
}

// mapStepResults converts domain.StepResult slice to []domain.StepSummary.
func mapStepResults(steps []domain.StepResult) []domain.StepSummary {
	out := make([]domain.StepSummary, len(steps))
	for i, sr := range steps {
		out[i] = domain.StepSummary{
			Index:    sr.Index,
			Action:   sr.Step.Action,
			Duration: sr.Duration,
			Success:  sr.Result != nil && sr.Result.Success,
		}
		if sr.Result != nil {
			out[i].Error = sr.Result.Error
			if sr.Result.Data != nil {
				out[i].Screenshot = sr.Result.Data
			}
		}
	}
	return out
}

// extractFinalURL returns the URL from the last step that has PageState.
func extractFinalURL(steps []domain.StepResult) string {
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].Result != nil && steps[i].Result.PageState != nil {
			return steps[i].Result.PageState.URL
		}
	}
	return ""
}

// extractFinalTitle returns the title from the last step that has PageState.
func extractFinalTitle(steps []domain.StepResult) string {
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].Result != nil && steps[i].Result.PageState != nil {
			return steps[i].Result.PageState.Title
		}
	}
	return ""
}

// resolveTimeout converts a seconds value to a Duration, falling back to default.
func resolveTimeout(seconds int) time.Duration {
	if seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return defaultTimeout
}

// compile-time interface check.
var _ domain.Bridge = (*OpenClawBridge)(nil)
