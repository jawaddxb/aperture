// Package api provides HTTP routing and handler wiring for the Aperture server.
// This file implements SSE task execution endpoints for the StatefulTaskPlanner.
package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/planner"
)

// TaskHandlers groups HTTP handlers for stateful task execution endpoints.
type TaskHandlers struct {
	taskPlanner   domain.TaskPlanner
	browserPool   domain.BrowserPool
	checkpointDir string
}

// NewTaskHandlers constructs TaskHandlers.
// checkpointDir is the directory used to save/resume task checkpoints.
func NewTaskHandlers(tp domain.TaskPlanner, pool domain.BrowserPool, checkpointDir string) *TaskHandlers {
	return &TaskHandlers{
		taskPlanner:   tp,
		browserPool:   pool,
		checkpointDir: checkpointDir,
	}
}

// ExecuteTaskRequest is the JSON body for POST /api/v1/tasks/execute.
type ExecuteTaskRequest struct {
	Goal      string `json:"goal"`
	Mode      string `json:"mode,omitempty"`       // research|hardened|max|auto
	SessionID string `json:"session_id,omitempty"` // optional imported session
}

// ResumeTaskRequest is the JSON body for POST /api/v1/tasks/resume.
type ResumeTaskRequest struct {
	CheckpointID string `json:"checkpoint_id"`
	Mode         string `json:"mode,omitempty"`
}

// Execute handles POST /api/v1/tasks/execute.
// Acquires a browser instance, runs the StatefulTaskPlanner, and streams
// TaskEvents as SSE to the client.
func (h *TaskHandlers) Execute(w http.ResponseWriter, r *http.Request) {
	var req ExecuteTaskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if req.Goal == "" {
		writeError(w, http.StatusBadRequest, "MISSING_GOAL", "field 'goal' is required")
		return
	}
	if req.Mode == "" || req.Mode == "auto" {
		req.Mode = "research"
	}

	if h.taskPlanner == nil {
		writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "task planner not configured")
		return
	}
	if h.browserPool == nil {
		writeError(w, http.StatusServiceUnavailable, "NO_BROWSER_POOL", "browser pool not configured")
		return
	}

	inst, err := h.browserPool.Acquire(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "BROWSER_UNAVAILABLE", err.Error())
		return
	}
	defer h.browserPool.Release(inst)

	streamTaskEvents(w, r, func(events chan<- domain.TaskEvent) (*domain.TaskContext, error) {
		return h.taskPlanner.PlanAndExecute(r.Context(), req.Goal, req.Mode, inst, events)
	})
}

// Resume handles POST /api/v1/tasks/resume.
// Loads a checkpoint and re-runs the task from the saved state.
func (h *TaskHandlers) Resume(w http.ResponseWriter, r *http.Request) {
	var req ResumeTaskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if req.CheckpointID == "" {
		writeError(w, http.StatusBadRequest, "MISSING_CHECKPOINT_ID", "field 'checkpoint_id' is required")
		return
	}

	if h.checkpointDir == "" {
		writeError(w, http.StatusServiceUnavailable, "NO_CHECKPOINT_DIR", "checkpoint directory not configured")
		return
	}

	taskCtx, err := planner.LoadCheckpoint(h.checkpointDir, req.CheckpointID)
	if err != nil {
		writeError(w, http.StatusNotFound, "CHECKPOINT_NOT_FOUND", err.Error())
		return
	}

	if h.taskPlanner == nil {
		writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "task planner not configured")
		return
	}
	if h.browserPool == nil {
		writeError(w, http.StatusServiceUnavailable, "NO_BROWSER_POOL", "browser pool not configured")
		return
	}

	mode := req.Mode
	if mode == "" {
		mode = taskCtx.Mode
	}
	goal := taskCtx.Goal

	inst, err := h.browserPool.Acquire(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "BROWSER_UNAVAILABLE", err.Error())
		return
	}
	defer h.browserPool.Release(inst)

	streamTaskEvents(w, r, func(events chan<- domain.TaskEvent) (*domain.TaskContext, error) {
		return h.taskPlanner.PlanAndExecute(r.Context(), goal, mode, inst, events)
	})
}

// streamTaskEvents sets SSE headers, runs fn, and writes TaskEvents as SSE lines.
func streamTaskEvents(
	w http.ResponseWriter,
	r *http.Request,
	fn func(events chan<- domain.TaskEvent) (*domain.TaskContext, error),
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	events := make(chan domain.TaskEvent, 64)
	done := make(chan struct{})

	// Goroutine: forward events to SSE writer.
	go func() {
		defer close(done)
		for ev := range events {
			data, err := json.Marshal(ev)
			if err != nil {
				slog.Warn("task event marshal error", "error", err)
				continue
			}
			if _, writeErr := fmt.Fprintf(w, "data: %s\n\n", data); writeErr != nil {
				slog.Warn("task sse write error", "error", writeErr)
				return
			}
			flusher.Flush()
		}
	}()

	_, runErr := fn(events)
	close(events)
	<-done

	if runErr != nil {
		slog.Warn("task execution error", "error", runErr)
	}
}
