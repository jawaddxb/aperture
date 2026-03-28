package captcha

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// ChainSolver tries multiple CAPTCHA solvers in sequence.
// If all automated solvers fail, it falls back to HITL (if configured).
type ChainSolver struct {
	solvers []domain.CaptchaSolver
	hitl    domain.HITLManager
}

// NewChainSolver creates a solver that tries each solver in order.
func NewChainSolver(solvers []domain.CaptchaSolver, hitl domain.HITLManager) *ChainSolver {
	return &ChainSolver{solvers: solvers, hitl: hitl}
}

func (c *ChainSolver) Name() string { return "chain" }

// Solve tries each solver in sequence. Falls back to HITL if all fail.
func (c *ChainSolver) Solve(ctx context.Context, ch domain.CaptchaChallenge) (*domain.CaptchaSolution, error) {
	for _, solver := range c.solvers {
		sol, err := solver.Solve(ctx, ch)
		if err == nil {
			slog.Info("captcha solved", "solver", solver.Name(), "type", ch.Type)
			return sol, nil
		}
		slog.Warn("captcha solver failed, trying next", "solver", solver.Name(), "type", ch.Type, "error", err)
	}

	// All automated solvers failed → HITL fallback
	if c.hitl != nil {
		slog.Info("captcha: all automated solvers failed, requesting human intervention", "type", ch.Type)
		resp, err := c.hitl.RequestIntervention(ctx, &domain.InterventionRequest{
			Prompt: fmt.Sprintf("CAPTCHA detected: %s on %s (site_key: %s)", ch.Type, ch.PageURL, ch.SiteKey),
			Type:   "captcha",
		})
		if err != nil {
			return nil, fmt.Errorf("captcha: HITL request failed: %w", err)
		}
		if resp != nil && resp.Data != "" {
			return &domain.CaptchaSolution{Token: resp.Data, SolvedBy: "human"}, nil
		}
		return nil, fmt.Errorf("captcha: HITL response empty")
	}

	return nil, fmt.Errorf("captcha: all solvers failed and no HITL configured")
}
