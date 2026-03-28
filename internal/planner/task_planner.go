// Package planner provides Planner implementations for Aperture.
// This file implements StatefulTaskPlanner — a multi-step orchestrator with
// pagination handling, re-planning on unexpected states, and checkpoint support.
package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/google/uuid"
)

// taskPlanResponse mirrors the JSON structure returned by the LLM for a task plan.
type taskPlanResponse struct {
	Steps              []taskPlanStep `json:"steps"`
	PaginationStrategy string         `json:"pagination_strategy"`
	EstimatedPages     int            `json:"estimated_pages"`
}

// taskPlanStep is a single step in the LLM-produced task plan.
type taskPlanStep struct {
	Action     string   `json:"action"`
	Target     string   `json:"target,omitempty"`
	Selector   string   `json:"selector,omitempty"`
	Fields     []string `json:"fields,omitempty"`
	Text       string   `json:"text,omitempty"`
	Reasoning  string   `json:"reasoning"`
	Completion string   `json:"completion,omitempty"`
}

// maxReplanAttempts limits how many consecutive re-plans are allowed to prevent loops.
const maxReplanAttempts = 3

// StatefulTaskPlanner executes multi-step goals with stateful context,
// pagination handling, and re-planning on unexpected page states.
type StatefulTaskPlanner struct {
	llm           domain.LLMClient
	registry      map[string]domain.Executor
	checkpointDir string
}

// NewStatefulTaskPlanner constructs a StatefulTaskPlanner.
// registry maps action names to their executor implementations.
// llm is used for planning and re-planning.
func NewStatefulTaskPlanner(llm domain.LLMClient, registry map[string]domain.Executor, checkpointDir string) *StatefulTaskPlanner {
	return &StatefulTaskPlanner{
		llm:           llm,
		registry:      registry,
		checkpointDir: checkpointDir,
	}
}

// PlanAndExecute decomposes goal into steps and executes them, emitting TaskEvents
// on events as progress is made. Satisfies domain.TaskPlanner.
func (p *StatefulTaskPlanner) PlanAndExecute(
	ctx context.Context,
	goal string,
	mode string,
	inst domain.BrowserInstance,
	events chan<- domain.TaskEvent,
) (*domain.TaskContext, error) {
	taskCtx := p.newTaskContext(goal, mode)
	emit(events, domain.TaskEvent{
		Type:    "progress",
		Message: "Planning task…",
	})

	plan, err := p.planTask(ctx, goal, "")
	if err != nil {
		taskCtx.Status = "failed"
		emitError(events, 0, 0, fmt.Sprintf("planning failed: %v", err))
		return taskCtx, fmt.Errorf("task planner: initial plan: %w", err)
	}

	taskCtx.TotalSteps = len(plan.Steps)
	taskCtx.Status = "executing"
	if plan.EstimatedPages > 1 {
		taskCtx.TotalPages = plan.EstimatedPages
		taskCtx.HasMore = true
	}

	slog.Info("task plan created",
		"task_id", taskCtx.ID,
		"steps", taskCtx.TotalSteps,
		"pagination", plan.PaginationStrategy,
		"est_pages", plan.EstimatedPages,
	)

	replanCount := 0
	stepIndex := 0

	for stepIndex < len(plan.Steps) {
		select {
		case <-ctx.Done():
			taskCtx.Status = "failed"
			emitError(events, stepIndex, len(plan.Steps), "context cancelled")
			return taskCtx, ctx.Err()
		default:
		}

		step := plan.Steps[stepIndex]
		taskCtx.CurrentStep = stepIndex + 1

		emit(events, domain.TaskEvent{
			Type:       "progress",
			Step:       taskCtx.CurrentStep,
			TotalSteps: len(plan.Steps),
			Message:    fmt.Sprintf("Step %d/%d: %s %s", taskCtx.CurrentStep, len(plan.Steps), step.Action, stepDescription(step)),
		})

		result, execErr := p.executeStep(ctx, inst, step)

		// Update page context in task state.
		if result != nil && result.PageState != nil {
			taskCtx.LastPageURL = result.PageState.URL
			taskCtx.LastPageTitle = result.PageState.Title
		}

		if execErr != nil || (result != nil && !result.Success) {
			errMsg := execErr.Error()
			if result != nil && result.Error != "" {
				errMsg = result.Error
			}

			slog.Warn("step failed, checking for replan",
				"task_id", taskCtx.ID,
				"step", stepIndex,
				"error", errMsg,
			)

			if replanCount >= maxReplanAttempts {
				taskCtx.Status = "failed"
				emitError(events, taskCtx.CurrentStep, len(plan.Steps), fmt.Sprintf("step %d failed after %d replan attempts: %s", stepIndex+1, maxReplanAttempts, errMsg))
				return taskCtx, fmt.Errorf("task planner: step %d failed: %s", stepIndex, errMsg)
			}

			// Re-plan from current position.
			emit(events, domain.TaskEvent{
				Type:       "replan",
				Step:       taskCtx.CurrentStep,
				TotalSteps: len(plan.Steps),
				Message:    fmt.Sprintf("Unexpected state, re-planning… (%s)", errMsg),
			})

			pageCtx := BuildPageContext(result.PageState, "", "")
			revisedPlan, replanErr := p.replanTask(ctx, goal, stepIndex, len(plan.Steps), pageCtx, errMsg)
			if replanErr != nil {
				taskCtx.Status = "failed"
				emitError(events, taskCtx.CurrentStep, len(plan.Steps), fmt.Sprintf("re-plan failed: %v", replanErr))
				return taskCtx, fmt.Errorf("task planner: replan: %w", replanErr)
			}

			replanCount++
			plan = revisedPlan
			taskCtx.TotalSteps = len(plan.Steps)
			stepIndex = 0

			slog.Info("task replanned",
				"task_id", taskCtx.ID,
				"new_steps", len(plan.Steps),
				"replan_attempt", replanCount,
			)
			continue
		}

		// Successful step — reset replan counter.
		replanCount = 0

		// Accumulate extracted data.
		if step.Action == "extract" && result != nil && len(result.Data) > 0 {
			taskCtx.Status = "extracting"
			extracted, parseErr := parseExtractedData(result.Data)
			if parseErr == nil {
				taskCtx.Extracted = append(taskCtx.Extracted, extracted...)
				taskCtx.ExtractCount += len(extracted)

				emit(events, domain.TaskEvent{
					Type:      "data",
					Step:      taskCtx.CurrentStep,
					Extracted: extracted,
					Count:     taskCtx.ExtractCount,
				})
			} else {
				slog.Warn("could not parse extracted data", "task_id", taskCtx.ID, "error", parseErr)
			}
		}

		// Record step result.
		taskCtx.StepResults = append(taskCtx.StepResults, domain.StepResult{
			Step: domain.Step{
				Action:    step.Action,
				Reasoning: step.Reasoning,
				Params:    stepToParams(step),
			},
			Result: result,
			Index:  stepIndex,
		})

		// Pagination bookkeeping.
		if isPaginationStep(step, plan.PaginationStrategy) {
			taskCtx.Status = "paginating"
			taskCtx.CurrentPage++
			if taskCtx.TotalPages > 0 && taskCtx.CurrentPage >= taskCtx.TotalPages {
				taskCtx.HasMore = false
			}
		}

		taskCtx.UpdatedAt = time.Now()

		// Checkpoint after each step.
		if p.checkpointDir != "" {
			if saveErr := SaveCheckpoint(p.checkpointDir, taskCtx); saveErr != nil {
				slog.Warn("checkpoint save failed", "task_id", taskCtx.ID, "error", saveErr)
			}
		}

		stepIndex++
	}

	taskCtx.Status = "completed"
	taskCtx.HasMore = false
	taskCtx.UpdatedAt = time.Now()

	emit(events, domain.TaskEvent{
		Type:       "complete",
		Step:       len(plan.Steps),
		TotalSteps: len(plan.Steps),
		Count:      taskCtx.ExtractCount,
	})

	slog.Info("task completed",
		"task_id", taskCtx.ID,
		"extract_count", taskCtx.ExtractCount,
	)

	return taskCtx, nil
}

// planTask calls the LLM to produce an initial task plan.
func (p *StatefulTaskPlanner) planTask(ctx context.Context, goal string, pageContext string) (*taskPlanResponse, error) {
	prompt := BuildTaskPlanPrompt(goal, pageContext)
	raw, err := p.llm.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("llm complete: %w", err)
	}
	return parseTaskPlanResponse(raw)
}

// replanTask calls the LLM to produce a revised plan from the current state.
func (p *StatefulTaskPlanner) replanTask(
	ctx context.Context,
	goal string,
	completedSteps, totalSteps int,
	pageContext string,
	unexpectedReason string,
) (*taskPlanResponse, error) {
	prompt := BuildReplanPrompt(goal, completedSteps, totalSteps, pageContext, unexpectedReason)
	raw, err := p.llm.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("llm complete: %w", err)
	}
	return parseTaskPlanResponse(raw)
}

// executeStep dispatches a single plan step to the appropriate executor.
func (p *StatefulTaskPlanner) executeStep(
	ctx context.Context,
	inst domain.BrowserInstance,
	step taskPlanStep,
) (*domain.ActionResult, error) {
	exec, ok := p.registry[step.Action]
	if !ok {
		return &domain.ActionResult{
			Action:  step.Action,
			Success: false,
			Error:   fmt.Sprintf("no executor registered for action %q", step.Action),
		}, fmt.Errorf("execute %s: no executor registered", step.Action)
	}
	params := stepToParams(step)
	result, err := exec.Execute(ctx, inst, params)
	if err != nil {
		return result, fmt.Errorf("execute %s: %w", step.Action, err)
	}
	if result != nil && !result.Success {
		return result, fmt.Errorf("execute %s: %s", step.Action, result.Error)
	}
	return result, nil
}

// newTaskContext initialises a fresh TaskContext.
func (p *StatefulTaskPlanner) newTaskContext(goal, mode string) *domain.TaskContext {
	now := time.Now()
	id := uuid.New().String()
	return &domain.TaskContext{
		ID:           id,
		Goal:         goal,
		Mode:         mode,
		Status:       "planning",
		CheckpointID: id,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// parseTaskPlanResponse strips markdown fences and unmarshals the LLM JSON.
func parseTaskPlanResponse(raw string) (*taskPlanResponse, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		lines := strings.SplitN(raw, "\n", 2)
		if len(lines) == 2 {
			raw = lines[1]
		}
		raw = strings.TrimSuffix(strings.TrimSpace(raw), "```")
		raw = strings.TrimSpace(raw)
	}

	var plan taskPlanResponse
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return nil, fmt.Errorf("parse task plan: %w", err)
	}
	if len(plan.Steps) == 0 {
		return nil, fmt.Errorf("parse task plan: no steps returned")
	}
	return &plan, nil
}

// stepToParams converts a taskPlanStep to an executor params map.
func stepToParams(step taskPlanStep) map[string]interface{} {
	params := map[string]interface{}{"action": step.Action}
	if step.Target != "" {
		params["target"] = step.Target
	}
	if step.Selector != "" {
		params["selector"] = step.Selector
	}
	if len(step.Fields) > 0 {
		params["fields"] = step.Fields
		// Build schema string for extract executor.
		params["schema"] = "Extract fields: " + strings.Join(step.Fields, ", ")
	}
	if step.Text != "" {
		params["text"] = step.Text
	}
	// For navigate, ensure url param is set from target.
	if step.Action == "navigate" && step.Target != "" {
		params["url"] = step.Target
	}
	return params
}

// stepDescription returns a short human-readable description of a step.
func stepDescription(step taskPlanStep) string {
	if step.Target != "" {
		return step.Target
	}
	if step.Selector != "" {
		return step.Selector
	}
	if step.Reasoning != "" {
		return step.Reasoning
	}
	return ""
}

// isPaginationStep returns true if the step corresponds to a pagination action.
func isPaginationStep(step taskPlanStep, strategy string) bool {
	switch strategy {
	case "click_next":
		return step.Action == "click" && (strings.Contains(strings.ToLower(step.Target), "next") ||
			strings.Contains(strings.ToLower(step.Reasoning), "next page"))
	case "url_param":
		return step.Action == "navigate" && strings.Contains(strings.ToLower(step.Reasoning), "page")
	case "scroll_load":
		return step.Action == "scroll"
	}
	return false
}

// parseExtractedData attempts to parse the raw extract bytes as a JSON array of items.
// Returns a slice of raw JSON messages for storage in TaskContext.Extracted.
func parseExtractedData(data []byte) ([]json.RawMessage, error) {
	raw := strings.TrimSpace(string(data))

	// Try as JSON array first.
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		return arr, nil
	}

	// Try as a single JSON object.
	var obj json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err == nil {
		return []json.RawMessage{obj}, nil
	}

	// Fall back to wrapping the raw string as a JSON string.
	wrapped, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("wrap raw: %w", err)
	}
	return []json.RawMessage{wrapped}, nil
}

// emit sends an event on the channel without blocking; drops silently if full.
func emit(ch chan<- domain.TaskEvent, ev domain.TaskEvent) {
	select {
	case ch <- ev:
	default:
	}
}

// emitError emits an error event.
func emitError(ch chan<- domain.TaskEvent, step, total int, msg string) {
	emit(ch, domain.TaskEvent{
		Type:       "error",
		Step:       step,
		TotalSteps: total,
		Error:      msg,
	})
}
