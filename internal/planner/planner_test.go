package planner_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/planner"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// mockLLMClient returns a fixed response string or an error.
type mockLLMClient struct {
	response string
	err      error
}

func (m *mockLLMClient) Complete(_ context.Context, _ string) (string, error) {
	return m.response, m.err
}

// stepsJSON encodes a slice of maps as JSON for use in mock LLM responses.
func stepsJSON(steps []map[string]interface{}) string {
	b, _ := json.Marshal(steps)
	return string(b)
}

// ─── StaticPlanner ────────────────────────────────────────────────────────────

func TestStaticPlanner_Navigate(t *testing.T) {
	p := planner.NewStaticPlanner()
	plan, err := p.Plan(context.Background(), "navigate to https://example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(plan.Steps))
	}
	s := plan.Steps[0]
	if s.Action != "navigate" {
		t.Errorf("expected action=navigate, got %q", s.Action)
	}
	if s.Params["url"] != "https://example.com" {
		t.Errorf("expected url=https://example.com, got %v", s.Params["url"])
	}
}

func TestStaticPlanner_NavigateGoTo(t *testing.T) {
	p := planner.NewStaticPlanner()
	plan, err := p.Plan(context.Background(), "go to https://go.dev", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Steps[0].Params["url"] != "https://go.dev" {
		t.Errorf("unexpected url: %v", plan.Steps[0].Params["url"])
	}
}

func TestStaticPlanner_Click(t *testing.T) {
	p := planner.NewStaticPlanner()
	plan, err := p.Plan(context.Background(), "click the submit button", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(plan.Steps))
	}
	s := plan.Steps[0]
	if s.Action != "click" {
		t.Errorf("expected action=click, got %q", s.Action)
	}
	if s.Params["target"] != "the submit button" {
		t.Errorf("unexpected target: %v", s.Params["target"])
	}
}

func TestStaticPlanner_ClickOn(t *testing.T) {
	p := planner.NewStaticPlanner()
	plan, err := p.Plan(context.Background(), "click on login", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Steps[0].Action != "click" {
		t.Errorf("expected click, got %q", plan.Steps[0].Action)
	}
}

func TestStaticPlanner_Type(t *testing.T) {
	p := planner.NewStaticPlanner()
	plan, err := p.Plan(context.Background(), "type hello@test.com into the email field", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(plan.Steps))
	}
	s := plan.Steps[0]
	if s.Action != "type" {
		t.Errorf("expected action=type, got %q", s.Action)
	}
	if s.Params["text"] != "hello@test.com" {
		t.Errorf("unexpected text: %v", s.Params["text"])
	}
	if s.Params["target"] != "the email field" {
		t.Errorf("unexpected target: %v", s.Params["target"])
	}
}

func TestStaticPlanner_Unhandled(t *testing.T) {
	p := planner.NewStaticPlanner()
	_, err := p.Plan(context.Background(), "fill in the login form with credentials", nil)
	if !errors.Is(err, planner.ErrUnhandled) {
		t.Errorf("expected ErrUnhandled, got %v", err)
	}
}

func TestStaticPlanner_Metadata(t *testing.T) {
	p := planner.NewStaticPlanner()
	plan, err := p.Plan(context.Background(), "navigate to https://example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Metadata["planner"] != "static" {
		t.Errorf("expected planner=static, got %q", plan.Metadata["planner"])
	}
}

// ─── LLMPlanner ───────────────────────────────────────────────────────────────

func TestLLMPlanner_ValidResponse(t *testing.T) {
	resp := stepsJSON([]map[string]interface{}{
		{"action": "navigate", "params": map[string]interface{}{"url": "https://example.com"}, "reasoning": "open page", "optional": false},
		{"action": "click", "params": map[string]interface{}{"target": "login"}, "reasoning": "click login", "optional": false},
	})
	client := &mockLLMClient{response: resp}
	p := planner.NewLLMPlanner(client)

	plan, err := p.Plan(context.Background(), "open example.com and click login", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(plan.Steps))
	}
	if plan.Steps[0].Action != "navigate" {
		t.Errorf("expected navigate, got %q", plan.Steps[0].Action)
	}
	if plan.Steps[1].Action != "click" {
		t.Errorf("expected click, got %q", plan.Steps[1].Action)
	}
}

func TestLLMPlanner_ClientError(t *testing.T) {
	client := &mockLLMClient{err: errors.New("timeout")}
	p := planner.NewLLMPlanner(client)

	_, err := p.Plan(context.Background(), "do something", nil)
	if err == nil {
		t.Fatal("expected error from failing client")
	}
}

func TestLLMPlanner_InvalidJSON(t *testing.T) {
	client := &mockLLMClient{response: "not json at all"}
	p := planner.NewLLMPlanner(client)

	_, err := p.Plan(context.Background(), "do something", nil)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestLLMPlanner_MarkdownFenced(t *testing.T) {
	raw := "```json\n" + stepsJSON([]map[string]interface{}{
		{"action": "screenshot", "params": map[string]interface{}{}, "reasoning": "capture", "optional": false},
	}) + "\n```"
	client := &mockLLMClient{response: raw}
	p := planner.NewLLMPlanner(client)

	plan, err := p.Plan(context.Background(), "take a screenshot", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Action != "screenshot" {
		t.Errorf("unexpected plan: %+v", plan)
	}
}

func TestLLMPlanner_WithPageState(t *testing.T) {
	var capturedPrompt string
	client := &capturingLLMClient{
		response: stepsJSON([]map[string]interface{}{
			{"action": "click", "params": map[string]interface{}{"target": "submit"}, "reasoning": "submit", "optional": false},
		}),
		capture: func(p string) { capturedPrompt = p },
	}
	p := planner.NewLLMPlanner(client)
	ps := &domain.PageState{URL: "https://example.com/login", Title: "Login Page"}
	_, err := p.Plan(context.Background(), "click submit", ps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsStr(capturedPrompt, "https://example.com/login") {
		t.Errorf("prompt missing URL; prompt: %q", capturedPrompt)
	}
}

// ─── CompositePlanner ─────────────────────────────────────────────────────────

func TestCompositePlanner_StaticFirst(t *testing.T) {
	client := &mockLLMClient{err: errors.New("should not be called")}
	p := planner.NewCompositePlanner(client)

	plan, err := p.Plan(context.Background(), "navigate to https://example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Metadata["planner"] != "static" {
		t.Errorf("expected static planner to handle this goal, got %q", plan.Metadata["planner"])
	}
}

func TestCompositePlanner_FallsBackToLLM(t *testing.T) {
	resp := stepsJSON([]map[string]interface{}{
		{"action": "type", "params": map[string]interface{}{"target": "email", "text": "a@b.com"}, "reasoning": "fill", "optional": false},
		{"action": "click", "params": map[string]interface{}{"target": "submit"}, "reasoning": "submit", "optional": false},
	})
	client := &mockLLMClient{response: resp}
	p := planner.NewCompositePlanner(client)

	plan, err := p.Plan(context.Background(), "fill in the login form with a@b.com and click submit", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Metadata["planner"] != "llm" {
		t.Errorf("expected llm planner fallback, got %q", plan.Metadata["planner"])
	}
	if len(plan.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(plan.Steps))
	}
}

// ─── test helpers ─────────────────────────────────────────────────────────────

type capturingLLMClient struct {
	response string
	capture  func(string)
}

func (c *capturingLLMClient) Complete(_ context.Context, prompt string) (string, error) {
	if c.capture != nil {
		c.capture(prompt)
	}
	return c.response, nil
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
