package planner_test

import (
	"context"
	"strings"
	"testing"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/planner"
)

// ─── BuildPageContext ─────────────────────────────────────────────────────────

// TestBuildPageContext_IncludesAllFields verifies all fields appear in the output.
func TestBuildPageContext_IncludesAllFields(t *testing.T) {
	ps := &domain.PageState{URL: "https://example.com/login", Title: "Login Page"}
	axSummary := "button: Submit\ninput: Email\ninput: Password"
	visionDesc := "A clean login form with email and password fields"

	ctx := planner.BuildPageContext(ps, axSummary, visionDesc)

	if !strings.Contains(ctx, "https://example.com/login") {
		t.Error("context missing URL")
	}
	if !strings.Contains(ctx, "Login Page") {
		t.Error("context missing Title")
	}
	if !strings.Contains(ctx, "Submit") {
		t.Error("context missing AX summary content")
	}
	if !strings.Contains(ctx, "clean login form") {
		t.Error("context missing vision description")
	}
}

// TestBuildPageContext_AXTruncation verifies AX summary is truncated at 2000 chars.
func TestBuildPageContext_AXTruncation(t *testing.T) {
	longAX := strings.Repeat("a", 2100)
	ctx := planner.BuildPageContext(nil, longAX, "")

	if strings.Contains(ctx, strings.Repeat("a", 2001)) {
		t.Error("AX summary should be truncated to 2000 chars")
	}
	if !strings.Contains(ctx, "[...truncated]") {
		t.Error("truncation marker should appear when AX summary is cut")
	}
}

// TestBuildPageContext_ExactBoundary verifies AX summary at exactly 2000 chars is not truncated.
func TestBuildPageContext_ExactBoundary(t *testing.T) {
	exact := strings.Repeat("b", 2000)
	ctx := planner.BuildPageContext(nil, exact, "")

	if strings.Contains(ctx, "[...truncated]") {
		t.Error("should not truncate exactly 2000 chars")
	}
}

// TestBuildPageContext_NilPageState verifies nil pageState is handled gracefully.
func TestBuildPageContext_NilPageState(t *testing.T) {
	ctx := planner.BuildPageContext(nil, "some AX", "some vision")
	if !strings.Contains(ctx, "some AX") {
		t.Error("AX summary should still appear with nil pageState")
	}
}

// ─── BuildUserPrompt ─────────────────────────────────────────────────────────

// TestBuildUserPrompt_ContainsGoal verifies the goal appears in the user prompt.
func TestBuildUserPrompt_ContainsGoal(t *testing.T) {
	p := planner.BuildUserPrompt("click the submit button", "")
	if !strings.Contains(p, "click the submit button") {
		t.Error("prompt missing goal")
	}
}

// TestBuildUserPrompt_WithPageContext verifies context appears in the prompt.
func TestBuildUserPrompt_WithPageContext(t *testing.T) {
	p := planner.BuildUserPrompt("click submit", "URL: https://example.com")
	if !strings.Contains(p, "URL: https://example.com") {
		t.Error("prompt missing page context")
	}
}

// TestBuildUserPrompt_WithoutContext verifies prompt works with empty context.
func TestBuildUserPrompt_WithoutContext(t *testing.T) {
	p := planner.BuildUserPrompt("navigate to google.com", "")
	if !strings.Contains(p, "navigate to google.com") {
		t.Error("prompt missing goal when no context")
	}
}

// ─── BuildFullPrompt ─────────────────────────────────────────────────────────

// TestBuildFullPrompt_ContainsPageContext verifies all page fields are included.
func TestBuildFullPrompt_ContainsPageContext(t *testing.T) {
	ps := &domain.PageState{URL: "https://app.example.com", Title: "Dashboard"}
	p := planner.BuildFullPrompt("take a screenshot", ps, "button: Logout", "Shows a dashboard")

	if !strings.Contains(p, "https://app.example.com") {
		t.Error("full prompt missing URL")
	}
	if !strings.Contains(p, "Dashboard") {
		t.Error("full prompt missing Title")
	}
	if !strings.Contains(p, "Logout") {
		t.Error("full prompt missing AX summary")
	}
	if !strings.Contains(p, "Shows a dashboard") {
		t.Error("full prompt missing vision description")
	}
}

// TestBuildFullPrompt_NoPageState verifies nil page state is handled gracefully.
func TestBuildFullPrompt_NoPageState(t *testing.T) {
	p := planner.BuildFullPrompt("do something", nil, "", "")
	if !strings.Contains(p, "do something") {
		t.Error("full prompt missing goal with nil page state")
	}
}

// ─── LLMPlanner with vision ───────────────────────────────────────────────────

// mockVisionAnalyzer returns a canned VisionResponse.
type mockVisionAnalyzer struct {
	description string
	err         error
}

func (m *mockVisionAnalyzer) Analyze(_ context.Context, _ *domain.VisionRequest) (*domain.VisionResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &domain.VisionResponse{Description: m.description}, nil
}

// TestLLMPlannerWithVision_IncludesVisionDesc verifies vision description
// appears in the prompt sent to the LLM.
func TestLLMPlannerWithVision_IncludesVisionDesc(t *testing.T) {
	var capturedPrompt string
	client := &capturingLLMClient{
		response: stepsJSON([]map[string]interface{}{
			{"action": "screenshot", "params": map[string]interface{}{}, "reasoning": "capture", "optional": false},
		}),
		capture: func(p string) { capturedPrompt = p },
	}
	va := &mockVisionAnalyzer{description: "A colorful homepage with a hero image"}
	p := planner.NewLLMPlannerWithVision(client, va)

	ps := &domain.PageState{URL: "https://example.com", Title: "Home"}
	_, err := p.Plan(context.Background(), "take a screenshot", ps)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if !strings.Contains(capturedPrompt, "A colorful homepage") {
		t.Errorf("prompt should include vision description; got: %q", capturedPrompt)
	}
}

// TestLLMPlannerWithVision_NilAnalyzer verifies nil vision analyzer does not break Plan.
func TestLLMPlannerWithVision_NilAnalyzer(t *testing.T) {
	client := &mockLLMClient{
		response: stepsJSON([]map[string]interface{}{
			{"action": "navigate", "params": map[string]interface{}{"url": "https://example.com"}, "reasoning": "open page", "optional": false},
		}),
	}
	p := planner.NewLLMPlannerWithVision(client, nil)
	_, err := p.Plan(context.Background(), "navigate to example.com", nil)
	if err != nil {
		t.Fatalf("Plan with nil vision: %v", err)
	}
}

// TestParseResponse_EdgeCases tests LLM response parsing edge cases via the planner.
func TestParseResponse_EdgeCases(t *testing.T) {
	cases := []struct {
		name     string
		response string
		wantErr  bool
	}{
		{
			name: "extra whitespace",
			response: "  \n  " + stepsJSON([]map[string]interface{}{
				{"action": "click", "params": map[string]interface{}{"target": "btn"}, "reasoning": "click", "optional": false},
			}) + "  \n  ",
			wantErr: false,
		},
		{
			name: "markdown json fence",
			response: "```json\n" + stepsJSON([]map[string]interface{}{
				{"action": "scroll", "params": map[string]interface{}{"direction": "down"}, "reasoning": "scroll", "optional": false},
			}) + "\n```",
			wantErr: false,
		},
		{
			name: "plain markdown fence",
			response: "```\n" + stepsJSON([]map[string]interface{}{
				{"action": "wait", "params": map[string]interface{}{"ms": 500}, "reasoning": "pause", "optional": false},
			}) + "\n```",
			wantErr: false,
		},
		{
			name:     "invalid json",
			response: `{not valid}`,
			wantErr:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &mockLLMClient{response: tc.response}
			p := planner.NewLLMPlanner(client)
			_, err := p.Plan(context.Background(), "do something", nil)
			if (err != nil) != tc.wantErr {
				t.Errorf("wantErr=%v, got err=%v", tc.wantErr, err)
			}
		})
	}
}
