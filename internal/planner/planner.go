// Package planner provides Planner implementations for decomposing natural-language
// goals into ordered sequences of concrete browser executor steps.
package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// ─── StaticPlanner ────────────────────────────────────────────────────────────

// StaticPlanner recognises common goal patterns via regex and returns a Plan
// without consulting an LLM. It is intentionally limited: unknown goals return
// ErrUnhandled so the caller can fall back to LLMPlanner.
type StaticPlanner struct{}

// NewStaticPlanner returns a StaticPlanner ready for use.
func NewStaticPlanner() *StaticPlanner {
	return &StaticPlanner{}
}

// navigateRe matches: "navigate to <url>" or "go to <url>".
var navigateRe = regexp.MustCompile(`(?i)^(?:navigate to|go to)\s+(\S+)$`)

// clickRe matches: "click <element>" or "click on <element>".
var clickRe = regexp.MustCompile(`(?i)^click(?:\s+on)?\s+(.+)$`)

// typeRe matches: "type <text> into <element>" or "type <text> in <element>".
var typeRe = regexp.MustCompile(`(?i)^type\s+(.+)\s+in(?:to)?\s+(.+)$`)

// ErrUnhandled is returned by StaticPlanner when the goal does not match any
// known pattern. Callers should fall back to LLMPlanner.
var ErrUnhandled = fmt.Errorf("static planner: goal not recognised")

// Plan attempts rule-based decomposition of goal.
// Returns ErrUnhandled when no rule matches.
func (p *StaticPlanner) Plan(_ context.Context, goal string, _ *domain.PageState) (*domain.Plan, error) {
	goal = strings.TrimSpace(goal)

	if m := navigateRe.FindStringSubmatch(goal); m != nil {
		return navigatePlan(goal, m[1]), nil
	}

	if m := typeRe.FindStringSubmatch(goal); m != nil {
		return typePlan(goal, m[1], m[2]), nil
	}

	if m := clickRe.FindStringSubmatch(goal); m != nil {
		return clickPlan(goal, m[1]), nil
	}

	return nil, ErrUnhandled
}

func navigatePlan(goal, url string) *domain.Plan {
	return &domain.Plan{
		Goal: goal,
		Steps: []domain.Step{
			{
				Action:    "navigate",
				Params:    map[string]interface{}{"url": url},
				Reasoning: "navigate to requested URL",
			},
		},
		Metadata: map[string]string{"complexity": "simple", "planner": "static"},
	}
}

func clickPlan(goal, target string) *domain.Plan {
	return &domain.Plan{
		Goal: goal,
		Steps: []domain.Step{
			{
				Action:    "click",
				Params:    map[string]interface{}{"target": strings.TrimSpace(target)},
				Reasoning: "click the requested element",
			},
		},
		Metadata: map[string]string{"complexity": "simple", "planner": "static"},
	}
}

func typePlan(goal, text, target string) *domain.Plan {
	return &domain.Plan{
		Goal: goal,
		Steps: []domain.Step{
			{
				Action: "type",
				Params: map[string]interface{}{
					"target": strings.TrimSpace(target),
					"text":   strings.TrimSpace(text),
				},
				Reasoning: "type the requested text into the target element",
			},
		},
		Metadata: map[string]string{"complexity": "simple", "planner": "static"},
	}
}

// ─── LLMPlanner ───────────────────────────────────────────────────────────────

// LLMPlanner sends the goal and current page state to an LLM and parses the
// JSON response into a Plan. It accepts a domain.LLMClient and an optional
// domain.VisionAnalyzer via constructor DI.
type LLMPlanner struct {
	client  domain.LLMClient
	vision  domain.VisionAnalyzer // optional; nil disables vision context
}

// NewLLMPlanner returns an LLMPlanner backed by client.
// Pass nil for vision to disable screenshot-based page context enrichment.
func NewLLMPlanner(client domain.LLMClient) *LLMPlanner {
	return &LLMPlanner{client: client}
}

// NewLLMPlannerWithVision returns an LLMPlanner that enriches prompts with
// vision analysis when pageState contains a screenshot.
func NewLLMPlannerWithVision(client domain.LLMClient, vision domain.VisionAnalyzer) *LLMPlanner {
	return &LLMPlanner{client: client, vision: vision}
}

// llmStep mirrors the JSON structure expected from the LLM response.
type llmStep struct {
	Action    string                 `json:"action"`
	Params    map[string]interface{} `json:"params"`
	Reasoning string                 `json:"reasoning"`
	Optional  bool                   `json:"optional"`
}

// Plan sends goal + pageState to the LLM and parses the JSON response.
// The method signature is backward-compatible with the domain.Planner interface.
func (p *LLMPlanner) Plan(ctx context.Context, goal string, pageState *domain.PageState) (*domain.Plan, error) {
	visionDesc := p.fetchVisionDesc(ctx, pageState)
	prompt := BuildFullPrompt(goal, pageState, "", visionDesc)
	raw, err := p.client.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("llm planner: complete: %w", err)
	}
	return parseResponse(goal, raw)
}

// fetchVisionDesc calls the VisionAnalyzer if available and returns the description.
// Returns an empty string when no analyzer is configured or the call fails.
func (p *LLMPlanner) fetchVisionDesc(ctx context.Context, pageState *domain.PageState) string {
	if p.vision == nil || pageState == nil {
		return ""
	}
	req := &domain.VisionRequest{
		Prompt:    "Describe the interactive elements and current page state",
		PageURL:   pageState.URL,
		PageTitle: pageState.Title,
	}
	resp, err := p.vision.Analyze(ctx, req)
	if err != nil || resp == nil {
		return ""
	}
	return resp.Description
}

func parseResponse(goal, raw string) (*domain.Plan, error) {
	raw = strings.TrimSpace(raw)

	// Strip markdown fences if present.
	if strings.HasPrefix(raw, "```") {
		lines := strings.SplitN(raw, "\n", 2)
		if len(lines) == 2 {
			raw = lines[1]
		}
		raw = strings.TrimSuffix(strings.TrimSpace(raw), "```")
		raw = strings.TrimSpace(raw)
	}

	var llmSteps []llmStep
	if err := json.Unmarshal([]byte(raw), &llmSteps); err != nil {
		return nil, fmt.Errorf("llm planner: parse response: %w", err)
	}

	steps := make([]domain.Step, len(llmSteps))
	for i, s := range llmSteps {
		steps[i] = domain.Step{
			Action:    s.Action,
			Params:    s.Params,
			Reasoning: s.Reasoning,
			Optional:  s.Optional,
		}
	}

	return &domain.Plan{
		Goal:     goal,
		Steps:    steps,
		Metadata: map[string]string{"complexity": "llm", "planner": "llm"},
	}, nil
}

// ─── CompositePlanner ─────────────────────────────────────────────────────────

// CompositePlanner tries StaticPlanner first and falls back to LLMPlanner for
// goals that the static rules do not recognise.
type CompositePlanner struct {
	static *StaticPlanner
	llm    *LLMPlanner
}

// NewCompositePlanner returns a CompositePlanner backed by the given LLM client.
func NewCompositePlanner(client domain.LLMClient) *CompositePlanner {
	return &CompositePlanner{
		static: NewStaticPlanner(),
		llm:    NewLLMPlanner(client),
	}
}

// Plan tries the static planner first; falls back to the LLM planner on ErrUnhandled.
func (p *CompositePlanner) Plan(ctx context.Context, goal string, pageState *domain.PageState) (*domain.Plan, error) {
	plan, err := p.static.Plan(ctx, goal, pageState)
	if err == nil {
		return plan, nil
	}
	if err != ErrUnhandled {
		return nil, err
	}
	return p.llm.Plan(ctx, goal, pageState)
}
