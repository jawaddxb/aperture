package planner

import (
	"context"
	"fmt"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/llm"
)

// FallbackPlanner is a no-op planner used when no LLM credentials are configured.
// It returns a single navigate step so basic URL tasks still work.
type FallbackPlanner struct{}

// NewFallbackPlanner constructs a FallbackPlanner.
func NewFallbackPlanner() *FallbackPlanner { return &FallbackPlanner{} }

// Plan returns a minimal one-step plan so the server can start without LLM keys.
func (f *FallbackPlanner) Plan(_ context.Context, goal string, _ *domain.PageState) (*domain.Plan, error) {
	return &domain.Plan{
		Goal:  goal,
		Steps: []domain.Step{{Action: "screenshot", Params: map[string]interface{}{}}},
	}, nil
}

// PlannerConfig holds constructor parameters for NewLLMPlannerFromConfig.
type PlannerConfig struct {
	Provider string
	APIKey   string
	Model    string
	BaseURL  string
}

// NewLLMPlannerFromConfig builds an LLMPlanner from a config struct.
// Returns an error if the underlying LLM client cannot be constructed.
func NewLLMPlannerFromConfig(cfg PlannerConfig) (*LLMPlanner, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("planner: API key is required")
	}
	client, err := llm.NewClient(llm.Config{
		Provider: cfg.Provider,
		Model:    cfg.Model,
		APIKey:   cfg.APIKey,
		BaseURL:  cfg.BaseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("planner: build llm client: %w", err)
	}
	return NewLLMPlanner(client), nil
}
