package planner_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/planner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── mock LLM client ──────────────────────────────────────────────────────────

type taskMockLLM struct {
	response string
	err      error
	calls    int
}

func (m *taskMockLLM) Complete(_ context.Context, _ string) (string, error) {
	m.calls++
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

// ─── mock executor ────────────────────────────────────────────────────────────

type taskMockExecutor struct {
	result  *domain.ActionResult
	err     error
	actions []string
}

func (m *taskMockExecutor) Execute(_ context.Context, _ domain.BrowserInstance, params map[string]interface{}) (*domain.ActionResult, error) {
	action, _ := params["action"].(string)
	m.actions = append(m.actions, action)
	if m.err != nil {
		return m.result, m.err
	}
	if m.result != nil {
		return m.result, nil
	}
	return &domain.ActionResult{
		Action:  action,
		Success: true,
		PageState: &domain.PageState{
			URL:   "https://example.com",
			Title: "Example",
		},
	}, nil
}

// ─── mock browser instance ────────────────────────────────────────────────────

type taskMockBrowser struct{}

func (m *taskMockBrowser) Context() context.Context          { return context.Background() }
func (m *taskMockBrowser) ID() string                        { return "mock-browser" }
func (m *taskMockBrowser) CreatedAt() time.Time              { return time.Now() }
func (m *taskMockBrowser) IsAlive() bool                     { return true }
func (m *taskMockBrowser) Close() error                      { return nil }
func (m *taskMockBrowser) Downloads() domain.DownloadManager { return nil }
func (m *taskMockBrowser) Network() domain.NetworkManager    { return nil }

// ─── helpers ──────────────────────────────────────────────────────────────────

func twoStepPlanJSON() string {
	return `{
		"steps": [
			{"action": "navigate", "target": "https://example.com", "reasoning": "Go to page", "completion": "URL contains example"},
			{"action": "extract", "selector": ".results", "fields": ["name"], "reasoning": "Extract data", "completion": "data.length > 0"}
		],
		"pagination_strategy": "none",
		"estimated_pages": 1
	}`
}

func extractResultJSON() []byte {
	data, _ := json.Marshal([]map[string]string{{"name": "Alice"}, {"name": "Bob"}})
	return data
}

// ─── tests ────────────────────────────────────────────────────────────────────

func TestStatefulTaskPlanner_PlanAndExecute_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	llm := &taskMockLLM{response: twoStepPlanJSON()}
	navExec := &taskMockExecutor{}
	extractExec := &taskMockExecutor{
		result: &domain.ActionResult{
			Action:  "extract",
			Success: true,
			Data:    extractResultJSON(),
			PageState: &domain.PageState{
				URL:   "https://example.com",
				Title: "Results",
			},
		},
	}

	registry := map[string]domain.Executor{
		"navigate": navExec,
		"extract":  extractExec,
	}

	dir := t.TempDir()
	tp := planner.NewStatefulTaskPlanner(llm, registry, dir, 0, 0)
	inst := &taskMockBrowser{}

	events := make(chan domain.TaskEvent, 64)
	taskCtx, err := tp.PlanAndExecute(context.Background(), "Extract data from example.com", "research", inst, events)
	close(events)

	require.NoError(t, err)
	assert.Equal(t, "completed", taskCtx.Status)
	assert.Equal(t, 2, taskCtx.ExtractCount)

	// Collect events.
	var evts []domain.TaskEvent
	for ev := range events {
		evts = append(evts, ev)
	}

	// Verify event types emitted.
	var types []string
	for _, e := range evts {
		types = append(types, e.Type)
	}
	assert.Contains(t, types, "progress")
	assert.Contains(t, types, "data")
	assert.Contains(t, types, "complete")
}

func TestStatefulTaskPlanner_PlanAndExecute_LLMError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	llm := &taskMockLLM{err: fmt.Errorf("llm unavailable")}
	registry := map[string]domain.Executor{}
	dir := t.TempDir()
	tp := planner.NewStatefulTaskPlanner(llm, registry, dir, 0, 0)
	inst := &taskMockBrowser{}

	events := make(chan domain.TaskEvent, 64)
	taskCtx, err := tp.PlanAndExecute(context.Background(), "Do something", "research", inst, events)
	close(events)

	assert.Error(t, err)
	assert.Equal(t, "failed", taskCtx.Status)

	var errEvents []domain.TaskEvent
	for ev := range events {
		if ev.Type == "error" {
			errEvents = append(errEvents, ev)
		}
	}
	assert.NotEmpty(t, errEvents)
}

func TestStatefulTaskPlanner_PlanAndExecute_UnknownAction(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	plan := `{"steps":[{"action":"unknown_action","target":"x","reasoning":"test"}],"pagination_strategy":"none","estimated_pages":1}`
	llm := &taskMockLLM{response: plan}
	registry := map[string]domain.Executor{} // empty — no executors

	// We need to allow replan to also fail gracefully.
	// After 3 replans all return same plan, the task should fail.
	replanCount := 0
	llm.response = plan
	_ = replanCount

	dir := t.TempDir()
	tp := planner.NewStatefulTaskPlanner(llm, registry, dir, 0, 0)
	inst := &taskMockBrowser{}

	events := make(chan domain.TaskEvent, 256)
	taskCtx, err := tp.PlanAndExecute(context.Background(), "unknown task", "research", inst, events)
	close(events)

	// After maxReplanAttempts the task must fail.
	assert.Error(t, err)
	assert.Equal(t, "failed", taskCtx.Status)
}

func TestStatefulTaskPlanner_Checkpoint_Written(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	llm := &taskMockLLM{response: twoStepPlanJSON()}
	navExec := &taskMockExecutor{}
	extractExec := &taskMockExecutor{
		result: &domain.ActionResult{
			Action:  "extract",
			Success: true,
			Data:    extractResultJSON(),
			PageState: &domain.PageState{
				URL:   "https://example.com",
				Title: "Results",
			},
		},
	}
	registry := map[string]domain.Executor{
		"navigate": navExec,
		"extract":  extractExec,
	}

	dir := t.TempDir()
	tp := planner.NewStatefulTaskPlanner(llm, registry, dir, 0, 0)
	inst := &taskMockBrowser{}

	events := make(chan domain.TaskEvent, 64)
	taskCtx, err := tp.PlanAndExecute(context.Background(), "test", "research", inst, events)
	close(events)
	for range events { /* drain */ }

	require.NoError(t, err)

	// A checkpoint file for the task ID should exist.
	loaded, loadErr := planner.LoadCheckpoint(dir, taskCtx.CheckpointID)
	require.NoError(t, loadErr)
	assert.Equal(t, taskCtx.ID, loaded.ID)
}
