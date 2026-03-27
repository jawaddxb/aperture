package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ApertureHQ/aperture/internal/api"
	"github.com/ApertureHQ/aperture/internal/domain"
)

// ─── stub Bridge ──────────────────────────────────────────────────────────────

type stubBridge struct {
	tasks    map[string]*domain.TaskResponse
	execErr  error
	cancelID string
}

func newStubBridge() *stubBridge {
	return &stubBridge{tasks: make(map[string]*domain.TaskResponse)}
}

func (b *stubBridge) ExecuteTask(_ context.Context, req *domain.TaskRequest) (*domain.TaskResponse, error) {
	if b.execErr != nil {
		return nil, b.execErr
	}
	id := req.ID
	if id == "" {
		id = "generated-id"
	}
	resp := &domain.TaskResponse{
		ID:         id,
		Success:    true,
		Goal:       req.Goal,
		FinalURL:   "https://example.com",
		FinalTitle: "Example",
		Duration:   10 * time.Millisecond,
		Steps: []domain.StepSummary{
			{Index: 0, Action: "navigate", Success: true, Duration: 5 * time.Millisecond},
		},
	}
	b.tasks[id] = resp
	return resp, nil
}

func (b *stubBridge) GetStatus(_ context.Context, taskID string) (*domain.TaskResponse, error) {
	resp, ok := b.tasks[taskID]
	if !ok {
		return nil, domain.ErrTaskNotFound
	}
	return resp, nil
}

func (b *stubBridge) CancelTask(_ context.Context, taskID string) error {
	if _, ok := b.tasks[taskID]; !ok {
		return domain.ErrTaskNotFound
	}
	b.cancelID = taskID
	return nil
}

// ─── stub BrowserPool ─────────────────────────────────────────────────────────

type stubPool struct{ available int }

func (p *stubPool) Acquire(_ context.Context, _ ...string) (domain.BrowserInstance, error) {
	return nil, nil
}
func (p *stubPool) Release(_ domain.BrowserInstance) {}
func (p *stubPool) Size() int                                                  { return 5 }
func (p *stubPool) Available() int                                             { return p.available }
func (p *stubPool) Close() error                                               { return nil }

// ─── helpers ──────────────────────────────────────────────────────────────────

func newBridgeRouter(bridge domain.Bridge, pool domain.BrowserPool) http.Handler {
	return api.NewRouter(api.RouterConfig{
		Bridge:      bridge,
		BrowserPool: pool,
	})
}

func bridgePost(t *testing.T, router http.Handler, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func bridgeGet(router http.Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func bridgeDelete(router http.Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

// ─── tests ────────────────────────────────────────────────────────────────────

func TestBridgeExecute_Sync_ReturnsFullResult(t *testing.T) {
	t.Parallel()
	bridge := newStubBridge()
	router := newBridgeRouter(bridge, nil)

	rr := bridgePost(t, router, "/api/v1/bridge/execute?sync=true", map[string]interface{}{
		"goal": "navigate to example.com",
		"id":   "task-sync-1",
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}

	var resp domain.TaskResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID != "task-sync-1" {
		t.Errorf("expected id=task-sync-1 got %q", resp.ID)
	}
	if !resp.Success {
		t.Errorf("expected success=true")
	}
	if resp.FinalURL == "" {
		t.Error("expected non-empty final_url")
	}
	if len(resp.Steps) == 0 {
		t.Error("expected at least one step in sync response")
	}
}

func TestBridgeExecute_Async_ReturnsTaskID(t *testing.T) {
	t.Parallel()
	bridge := newStubBridge()
	router := newBridgeRouter(bridge, nil)

	rr := bridgePost(t, router, "/api/v1/bridge/execute", map[string]interface{}{
		"goal": "click button",
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	taskID, ok := resp["task_id"].(string)
	if !ok || taskID == "" {
		t.Errorf("expected non-empty task_id in async response, got: %v", resp)
	}

	// GET should return the stored result.
	getResp := bridgeGet(router, "/api/v1/bridge/tasks/"+taskID)
	if getResp.Code != http.StatusOK {
		t.Fatalf("GET task expected 200 got %d: %s", getResp.Code, getResp.Body.String())
	}

	var taskResp domain.TaskResponse
	if err := json.NewDecoder(getResp.Body).Decode(&taskResp); err != nil {
		t.Fatalf("decode task: %v", err)
	}
	if taskResp.ID != taskID {
		t.Errorf("expected task id %q got %q", taskID, taskResp.ID)
	}
}

func TestBridgeCancelTask(t *testing.T) {
	t.Parallel()
	bridge := newStubBridge()
	router := newBridgeRouter(bridge, nil)

	// Seed a task.
	bridgePost(t, router, "/api/v1/bridge/execute?sync=true", map[string]interface{}{
		"goal": "navigate",
		"id":   "cancel-task-1",
	})

	rr := bridgeDelete(router, "/api/v1/bridge/tasks/cancel-task-1")
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204 got %d: %s", rr.Code, rr.Body.String())
	}
	if bridge.cancelID != "cancel-task-1" {
		t.Errorf("expected cancelID=cancel-task-1 got %q", bridge.cancelID)
	}
}

func TestBridgeCancelTask_NotFound(t *testing.T) {
	t.Parallel()
	router := newBridgeRouter(newStubBridge(), nil)

	rr := bridgeDelete(router, "/api/v1/bridge/tasks/ghost-id")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 got %d", rr.Code)
	}
}

func TestBridgeHealth_ReturnsStatus(t *testing.T) {
	t.Parallel()
	pool := &stubPool{available: 3}
	router := newBridgeRouter(newStubBridge(), pool)

	rr := bridgeGet(router, "/api/v1/bridge/health")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status=ok got %v", resp["status"])
	}
	if resp["browser_pool"] != "available" {
		t.Errorf("expected browser_pool=available got %v", resp["browser_pool"])
	}
	if _, ok := resp["active_tasks"]; !ok {
		t.Error("expected active_tasks field in health response")
	}
}

func TestBridgeHealth_NoBridge_ReturnsDegraded(t *testing.T) {
	t.Parallel()
	router := api.NewRouter(api.RouterConfig{}) // no bridge

	rr := bridgeGet(router, "/api/v1/bridge/health")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "degraded" {
		t.Errorf("expected status=degraded when bridge nil, got %v", resp["status"])
	}
}

func TestBridgeQuick_ReturnsSyncResult(t *testing.T) {
	t.Parallel()
	router := newBridgeRouter(newStubBridge(), nil)

	rr := bridgePost(t, router, "/api/v1/bridge/quick", map[string]interface{}{
		"goal": "take a screenshot of google.com",
		"url":  "https://google.com",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}

	var resp domain.TaskResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Success {
		t.Errorf("expected success=true")
	}
}

func TestBridgeExecute_MissingGoal_Returns400(t *testing.T) {
	t.Parallel()
	router := newBridgeRouter(newStubBridge(), nil)

	rr := bridgePost(t, router, "/api/v1/bridge/execute?sync=true", map[string]interface{}{})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", rr.Code)
	}
}
