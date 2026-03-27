// Package resolver contains element resolution strategies for Aperture.
// This file implements UnifiedResolver: Tier 1 AX-tree + Tier 2 DOM heuristic fallback.
package resolver

import (
	"context"
	"fmt"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// confidenceAutoSelect is the threshold above which a single candidate is returned.
const confidenceAutoSelect = 0.9

// confidenceMinUsable is the minimum confidence for an AX result to be included.
// Below this threshold the AX result is discarded and DOM-only results are used.
const confidenceMinUsable = 0.5

// unifiedResolver composes an AX-tree resolver and a DOM resolver into a single
// tiered resolution pipeline. It implements domain.UnifiedResolver.
// Safe for single-goroutine use per instance.
type unifiedResolver struct {
	axFactory  func(browserCtx context.Context) *AXTreeResolver
	domFactory func(browserCtx context.Context) *DOMResolver
}

// NewUnifiedResolver constructs a UnifiedResolver with default factories.
// Dependency injection: both factory functions are swappable for testing.
func NewUnifiedResolver() domain.UnifiedResolver {
	return &unifiedResolver{
		axFactory:  NewAXTreeResolver,
		domFactory: NewDOMResolver,
	}
}

// NewUnifiedResolverWithFactories constructs a UnifiedResolver with custom factories.
// Use in tests to inject mock or stub resolvers.
func NewUnifiedResolverWithFactories(
	axFactory func(context.Context) *AXTreeResolver,
	domFactory func(context.Context) *DOMResolver,
) domain.UnifiedResolver {
	return &unifiedResolver{
		axFactory:  axFactory,
		domFactory: domFactory,
	}
}

// Resolve finds elements matching target in inst using AX-tree first, DOM second.
//
// Decision tree:
//   - AX confidence > 0.9  → single auto-selected candidate, tier=ax_tree
//   - AX confidence 0.5–0.9 → AX candidates merged with DOM, ranked by confidence
//   - AX confidence < 0.5 or no AX match → DOM-only results, tier=dom_heuristic
//
// Returns *domain.ErrNoMatch when no candidates are found.
// Implements domain.UnifiedResolver.
func (u *unifiedResolver) Resolve(
	ctx context.Context,
	target domain.ResolutionTarget,
	inst domain.BrowserInstance,
) (*domain.Resolution, error) {
	browserCtx := inst.Context()

	axResults, err := u.resolveAX(ctx, browserCtx, target)
	if err != nil {
		return nil, fmt.Errorf("unified resolver ax: %w", err)
	}

	topAXConfidence := topConfidence(axResults)

	// Fast path: high-confidence AX hit → auto-select.
	if topAXConfidence > confidenceAutoSelect {
		candidates := toCandidates(axResults[:1], domain.TierAXTree)
		return &domain.Resolution{
			Tier:       domain.TierAXTree,
			Confidence: candidates[0].Confidence,
			Candidates: candidates,
		}, nil
	}

	domResults, err := u.resolveDOM(ctx, browserCtx, target)
	if err != nil {
		return nil, fmt.Errorf("unified resolver dom: %w", err)
	}

	return u.merge(axResults, domResults, topAXConfidence)
}

// resolveAX runs Snapshot then searches by role/text via the AX resolver.
func (u *unifiedResolver) resolveAX(
	ctx context.Context,
	browserCtx context.Context,
	target domain.ResolutionTarget,
) ([]domain.MatchResult, error) {
	ax := u.axFactory(browserCtx)
	if _, err := ax.Snapshot(ctx); err != nil {
		// AX snapshot failure is non-fatal; we fall through to DOM.
		return nil, nil
	}
	role := target.Role
	name := target.Text
	results, err := ax.FindByRole(role, name)
	if err != nil {
		return nil, err
	}
	sortMatchResults(results)
	return results, nil
}

// resolveDOM queries the DOM resolver using text, selector, and common patterns.
func (u *unifiedResolver) resolveDOM(
	ctx context.Context,
	browserCtx context.Context,
	target domain.ResolutionTarget,
) ([]domain.MatchResult, error) {
	dom := u.domFactory(browserCtx)

	var all []domain.MatchResult

	if target.Text != "" {
		results, err := dom.FindByText(ctx, target.Text)
		if err != nil {
			return nil, err
		}
		all = append(all, results...)
	}

	if target.Selector != "" {
		results, err := dom.FindBySelector(ctx, target.Selector)
		if err != nil {
			return nil, err
		}
		all = append(all, results...)
	}

	if len(all) == 0 {
		results, err := dom.FindByPattern(ctx, CommonPatterns)
		if err != nil {
			return nil, err
		}
		all = append(all, results...)
	}

	sortMatchResults(all)
	return all, nil
}

// merge combines AX and DOM results according to the confidence thresholds.
func (u *unifiedResolver) merge(
	axResults []domain.MatchResult,
	domResults []domain.MatchResult,
	topAXConfidence float64,
) (*domain.Resolution, error) {
	var combined []domain.Candidate

	// Include AX results only when they meet the minimum usable threshold.
	if topAXConfidence >= confidenceMinUsable {
		combined = append(combined, toCandidates(axResults, domain.TierAXTree)...)
	}
	combined = append(combined, toCandidates(domResults, domain.TierDOMHeuristic)...)

	if len(combined) == 0 {
		return nil, &domain.ErrNoMatch{}
	}

	sortCandidates(combined)

	tier := combined[0].ResolutionTier
	return &domain.Resolution{
		Tier:       tier,
		Confidence: combined[0].Confidence,
		Candidates: combined,
	}, nil
}

// Helpers (toCandidates, topConfidence, sortMatchResults, sortCandidates,
// cannedUnifiedResolver and its methods) live in resolver_helpers.go.
