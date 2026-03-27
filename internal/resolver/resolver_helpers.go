// Package resolver contains element resolution strategies for Aperture.
// This file contains shared conversion/sort helpers and the cannedUnifiedResolver
// test helper, extracted to keep resolver.go within LOC limits.
package resolver

import (
	"context"
	"sort"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// ─── conversion helpers ────────────────────────────────────────────────────────

// toCandidates converts MatchResult slice to Candidate slice with the given tier.
func toCandidates(results []domain.MatchResult, tier domain.ResolutionTier) []domain.Candidate {
	out := make([]domain.Candidate, 0, len(results))
	for _, r := range results {
		if r.Node == nil {
			continue
		}
		out = append(out, domain.Candidate{
			SemanticID:     r.Node.SemanticID,
			Role:           r.Node.Role,
			Name:           r.Node.Name,
			Confidence:     r.Confidence,
			ResolutionTier: tier,
		})
	}
	return out
}

// topConfidence returns the highest confidence from results, or 0.
func topConfidence(results []domain.MatchResult) float64 {
	var top float64
	for _, r := range results {
		if r.Confidence > top {
			top = r.Confidence
		}
	}
	return top
}

// sortMatchResults sorts MatchResults by confidence descending, in-place.
func sortMatchResults(results []domain.MatchResult) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Confidence > results[j].Confidence
	})
}

// sortCandidates sorts Candidates by confidence descending, in-place.
func sortCandidates(candidates []domain.Candidate) {
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Confidence > candidates[j].Confidence
	})
}

// ─── cannedUnifiedResolver ─────────────────────────────────────────────────────

// cannedUnifiedResolver is a test helper that delegates to arbitrary ElementResolver
// and DOMElementResolver implementations instead of concrete AXTreeResolver/DOMResolver.
// Exported only for use in tests via NewUnifiedResolverWithCanned.
type cannedUnifiedResolver struct {
	ax  domain.ElementResolver
	dom domain.DOMElementResolver
}

// NewUnifiedResolverWithCanned creates a UnifiedResolver backed by arbitrary
// ElementResolver and DOMElementResolver implementations.
// Intended for unit tests that need to inject stub/mock resolvers.
func NewUnifiedResolverWithCanned(ax domain.ElementResolver, dom domain.DOMElementResolver) domain.UnifiedResolver {
	return &cannedUnifiedResolver{ax: ax, dom: dom}
}

// Resolve implements domain.UnifiedResolver using the injected resolvers.
func (c *cannedUnifiedResolver) Resolve(
	ctx context.Context,
	target domain.ResolutionTarget,
	_ domain.BrowserInstance,
) (*domain.Resolution, error) {
	axResults := c.cannedAXPhase(ctx, target)
	topAX := topConfidence(axResults)

	// Fast path: high-confidence AX hit → auto-select.
	if topAX > confidenceAutoSelect {
		candidates := toCandidates(axResults[:1], domain.TierAXTree)
		return &domain.Resolution{
			Tier:       domain.TierAXTree,
			Confidence: candidates[0].Confidence,
			Candidates: candidates,
		}, nil
	}

	domResults := c.cannedDOMPhase(ctx, target)
	return c.cannedMerge(axResults, domResults, topAX, target)
}

// cannedAXPhase runs snapshot + FindByRole via the canned AX resolver.
// Errors are silently swallowed (non-fatal; DOM fallback covers the gap).
func (c *cannedUnifiedResolver) cannedAXPhase(
	ctx context.Context,
	target domain.ResolutionTarget,
) []domain.MatchResult {
	var results []domain.MatchResult
	if _, err := c.ax.Snapshot(ctx); err == nil {
		results, _ = c.ax.FindByRole(target.Role, target.Text)
	}
	sortMatchResults(results)
	return results
}

// cannedDOMPhase queries text, selector, and common patterns via the canned DOM resolver.
// Errors are silently swallowed (non-fatal).
func (c *cannedUnifiedResolver) cannedDOMPhase(
	ctx context.Context,
	target domain.ResolutionTarget,
) []domain.MatchResult {
	var all []domain.MatchResult
	if target.Text != "" {
		results, _ := c.dom.FindByText(ctx, target.Text)
		all = append(all, results...)
	}
	if target.Selector != "" {
		results, _ := c.dom.FindBySelector(ctx, target.Selector)
		all = append(all, results...)
	}
	if len(all) == 0 {
		results, _ := c.dom.FindByPattern(ctx, CommonPatterns)
		all = append(all, results...)
	}
	sortMatchResults(all)
	return all
}

// cannedMerge combines AX and DOM candidates and builds the Resolution.
func (c *cannedUnifiedResolver) cannedMerge(
	axResults []domain.MatchResult,
	domResults []domain.MatchResult,
	topAX float64,
	target domain.ResolutionTarget,
) (*domain.Resolution, error) {
	var combined []domain.Candidate
	if topAX >= confidenceMinUsable {
		combined = append(combined, toCandidates(axResults, domain.TierAXTree)...)
	}
	combined = append(combined, toCandidates(domResults, domain.TierDOMHeuristic)...)
	if len(combined) == 0 {
		return nil, &domain.ErrNoMatch{Target: target}
	}
	sortCandidates(combined)
	return &domain.Resolution{
		Tier:       combined[0].ResolutionTier,
		Confidence: combined[0].Confidence,
		Candidates: combined,
	}, nil
}
